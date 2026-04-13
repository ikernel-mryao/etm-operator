package config

import "time"

const (
	// API group and version
	APIGroup   = "etmem.openeuler.io"
	APIVersion = "v1alpha1"

	// etmemd socket
	DefaultSocketPath = "/var/run/etmemd/etmemd.sock"
	DefaultSocketName = "etmemd_socket"

	// etmem binary
	EtmemBinaryPath = "/usr/bin/etmem"

	// Agent reconcile interval
	DefaultReconcileInterval = 30 * time.Second

	// 宿主机路径约定：Agent 容器通过 hostPath 挂载访问宿主机资源
	// HostProcPath 用于读取进程信息（/proc/<pid>/comm, /proc/<pid>/status）
	// HostCgroupPath 用于读取 cgroup 层级（memory/kubepods/pod<uid>/cgroup.procs）
	HostProcPath   = "/host/proc"
	HostCgroupPath = "/host/sys/fs/cgroup"

	// Default profile
	DefaultProfile = "moderate"

	// Default circuit breaker thresholds
	DefaultPodRestartThreshold = 5
	DefaultNodePSIThreshold    = 70

	// Config file temp directory (inside Agent container)
	EtmemConfigDir = "/tmp/etmem-configs"

	// Project name format: <namespace>-<podName>
	// Max 64 chars per etmem constraint
	ProjectNameMaxLen = 64

	// Process name max 15 chars per etmem constraint
	ProcessNameMaxLen = 15
)
