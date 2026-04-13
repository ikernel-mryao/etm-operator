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
