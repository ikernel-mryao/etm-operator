package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildCgroupRelPath 构造 Kubernetes Pod 的 cgroup v1 相对路径。
// cgroup v1 cgroupfs 驱动路径约定：memory/kubepods/<qos>/pod<uid>/
// QoS 类映射：Guaranteed → 无中间层，Burstable → burstable/，BestEffort → besteffort/
func BuildCgroupRelPath(podUID string, qosClass string) string {
	// Normalize QoS class to cgroup dir name
	var qosDir string
	switch strings.ToLower(qosClass) {
	case "guaranteed":
		qosDir = ""
	case "burstable":
		qosDir = "burstable/"
	default:
		qosDir = "besteffort/"
	}
	// cgroup v1 cgroupfs driver path: memory/kubepods/<qos>/pod<uid>/
	if qosDir != "" {
		return fmt.Sprintf("memory/kubepods/%spod%s", qosDir, podUID)
	}
	return fmt.Sprintf("memory/kubepods/pod%s", podUID)
}

type ResolvedProcess struct {
	PID   int
	Name  string
	RSSKB uint64
}

type PIDResolver struct {
	procRoot   string
	cgroupRoot string
}

func NewPIDResolver(procRoot, cgroupRoot string) *PIDResolver {
	return &PIDResolver{procRoot: procRoot, cgroupRoot: cgroupRoot}
}

func (r *PIDResolver) ResolvePIDs(cgroupRelPath string, processNames []string) ([]ResolvedProcess, error) {
	procsPath := filepath.Join(r.cgroupRoot, cgroupRelPath, "cgroup.procs")
	pids, err := r.readCgroupProcs(procsPath)
	if err != nil {
		return nil, fmt.Errorf("read cgroup.procs at %s: %w", procsPath, err)
	}
	nameSet := make(map[string]bool, len(processNames))
	for _, n := range processNames {
		nameSet[n] = true
	}
	var result []ResolvedProcess
	for _, pid := range pids {
		comm, err := r.readComm(pid)
		if err != nil {
			continue
		}
		if !nameSet[comm] {
			continue
		}
		rss, _ := r.ReadRSSKB(pid)
		result = append(result, ResolvedProcess{PID: pid, Name: comm, RSSKB: rss})
	}
	return result, nil
}

func (r *PIDResolver) readCgroupProcs(path string) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pids []int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, scanner.Err()
}

func (r *PIDResolver) readComm(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (r *PIDResolver) ReadRSSKB(pid int) (uint64, error) {
	path := filepath.Join(r.procRoot, strconv.Itoa(pid), "status")
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return val, nil
			}
		}
	}
	return 0, fmt.Errorf("VmRSS not found in %s", path)
}
