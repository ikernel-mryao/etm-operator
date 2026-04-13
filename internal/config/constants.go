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

	// Host path mounts inside Agent container
	HostProcPath   = "/host/proc"
	HostCgroupPath = "/host/sys/fs/cgroup"

	// Default profile
	DefaultProfile = "moderate"

	// Default circuit breaker thresholds
	DefaultPodRestartThreshold     = 3
	DefaultNodePSIThresholdPercent = 80

	// Config file temp directory (inside Agent container)
	EtmemConfigDir = "/tmp/etmem-configs"

	// Project name format: <namespace>-<podName>
	// Max 64 chars per etmem constraint
	ProjectNameMaxLen = 64

	// Process name max 15 chars per etmem constraint
	ProcessNameMaxLen = 15
)
