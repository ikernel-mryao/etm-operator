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
// 同时支持 cgroupfs 和 systemd 两种 cgroup 驱动：
//   - cgroupfs: memory/kubepods/<qos>/pod<uid>/
//   - systemd:  memory/kubepods.slice/kubepods-<qos>.slice/kubepods-<qos>-pod<uid_underscored>.slice/
//
// 通过探测 cgroupRoot 下是否存在 "kubepods.slice" 目录来自动选择驱动模式。
func BuildCgroupRelPath(podUID string, qosClass string) string {
	return buildCgroupRelPathForDriver(podUID, qosClass, false)
}

// BuildCgroupRelPathSystemd 明确使用 systemd cgroup 驱动路径格式。
func BuildCgroupRelPathSystemd(podUID string, qosClass string) string {
	return buildCgroupRelPathForDriver(podUID, qosClass, true)
}

func buildCgroupRelPathForDriver(podUID string, qosClass string, systemd bool) string {
	if systemd {
		return buildSystemdCgroupPath(podUID, qosClass)
	}
	return buildCgroupfsPath(podUID, qosClass)
}

// systemd 驱动：UID 中的 '-' 需替换为 '_'，路径使用 .slice 后缀
func buildSystemdCgroupPath(podUID string, qosClass string) string {
	uidEscaped := strings.ReplaceAll(podUID, "-", "_")
	switch strings.ToLower(qosClass) {
	case "guaranteed":
		return fmt.Sprintf("memory/kubepods.slice/kubepods-pod%s.slice", uidEscaped)
	case "burstable":
		return fmt.Sprintf("memory/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod%s.slice", uidEscaped)
	default:
		return fmt.Sprintf("memory/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod%s.slice", uidEscaped)
	}
}

func buildCgroupfsPath(podUID string, qosClass string) string {
	var qosDir string
	switch strings.ToLower(qosClass) {
	case "guaranteed":
		qosDir = ""
	case "burstable":
		qosDir = "burstable/"
	default:
		qosDir = "besteffort/"
	}
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

// ResolvePIDs 收集 pod cgroup 下所有 PID（含容器子 cgroup），然后按进程名过滤。
// cgroup v1 中 pod 级 cgroup.procs 通常为空，PID 位于 cri-containerd-*.scope 等子目录。
func (r *PIDResolver) ResolvePIDs(cgroupRelPath string, processNames []string) ([]ResolvedProcess, error) {
	podCgroupDir := filepath.Join(r.cgroupRoot, cgroupRelPath)
	pids, err := r.collectAllPIDs(podCgroupDir)
	if err != nil {
		return nil, fmt.Errorf("collect PIDs under %s: %w", podCgroupDir, err)
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

// collectAllPIDs 读取目录自身及直接子目录中的 cgroup.procs，合并所有 PID。
func (r *PIDResolver) collectAllPIDs(dir string) ([]int, error) {
	var allPIDs []int

	// 读取当前目录的 cgroup.procs
	if pids, err := r.readCgroupProcs(filepath.Join(dir, "cgroup.procs")); err == nil {
		allPIDs = append(allPIDs, pids...)
	}

	// 遍历直接子目录（容器 scope），读取各自的 cgroup.procs
	entries, err := os.ReadDir(dir)
	if err != nil {
		if len(allPIDs) > 0 {
			return allPIDs, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		childProcs := filepath.Join(dir, entry.Name(), "cgroup.procs")
		if pids, err := r.readCgroupProcs(childProcs); err == nil {
			allPIDs = append(allPIDs, pids...)
		}
	}
	return allPIDs, nil
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
