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

	// etmemd 在宿主机读取配置文件，路径必须宿主机和 Agent 容器都可访问。
	// Agent 通过 hostPath 将此目录挂载到容器同一路径，确保 etmemd 能找到文件。
	EtmemConfigDir = "/var/run/etmem/configs"

	// Project name format: <namespace>-<podName>
	// Max 64 chars per etmem constraint
	ProjectNameMaxLen = 64

	// Process name max 15 chars per etmem constraint
	ProcessNameMaxLen = 15
)
