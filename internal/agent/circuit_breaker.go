// CircuitBreaker 实现两级熔断保护机制，避免 etmem 内存回收导致业务受损。
// Pod 级熔断：容器 OOMKilled 或重启次数超阈值 → 跳过该 Pod
// 节点级熔断：内存 PSI avg10 超阈值 → 停止节点所有 etmem 任务
// 设计原则：熔断后不自动恢复，需人工介入排查后手动重启（spec.suspend）。
package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type CircuitBreaker struct {
	podRestartThreshold int
	nodePSIThreshold    int
	psiDir              string
}

func NewCircuitBreaker(podRestartThreshold, nodePSIThreshold int, psiDir string) *CircuitBreaker {
	if psiDir == "" {
		psiDir = "/proc/pressure"
	}
	return &CircuitBreaker{
		podRestartThreshold: podRestartThreshold,
		nodePSIThreshold:    nodePSIThreshold,
		psiDir:              psiDir,
	}
}

func (cb *CircuitBreaker) IsPodTripped(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if int(cs.RestartCount) >= cb.podRestartThreshold {
			return true
		}
		if cs.LastTerminationState.Terminated != nil &&
			cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			return true
		}
	}
	return false
}

func (cb *CircuitBreaker) IsNodeTripped() (bool, int) {
	psiPath := filepath.Join(cb.psiDir, "memory")
	psi, err := readPSIAvg10(psiPath)
	if err != nil {
		return false, 0
	}
	return psi >= cb.nodePSIThreshold, psi
}

func readPSIAvg10(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "some ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "avg10=") {
				valStr := strings.TrimPrefix(field, "avg10=")
				val, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return 0, err
				}
				return int(val), nil
			}
		}
	}
	return 0, fmt.Errorf("avg10 not found in %s", path)
}
