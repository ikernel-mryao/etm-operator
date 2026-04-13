package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockProc(t *testing.T, pids map[int]string) string {
	t.Helper()
	procRoot := t.TempDir()
	for pid, comm := range pids {
		pidDir := filepath.Join(procRoot, strconv.Itoa(pid))
		require.NoError(t, os.MkdirAll(pidDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "comm"), []byte(comm+"\n"), 0644))
		statusContent := "Name:\t" + comm + "\nVmRSS:\t1048576 kB\n"
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "status"), []byte(statusContent), 0644))
	}
	return procRoot
}

func setupMockCgroup(t *testing.T, pids []int) string {
	t.Helper()
	cgroupRoot := t.TempDir()
	cgroupPath := filepath.Join(cgroupRoot, "kubepods/pod-abc/container-xyz")
	require.NoError(t, os.MkdirAll(cgroupPath, 0755))
	var content string
	for _, pid := range pids {
		content += strconv.Itoa(pid) + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(content), 0644))
	return cgroupRoot
}

func TestPIDResolver_ResolvePIDs_MatchByName(t *testing.T) {
	procRoot := setupMockProc(t, map[int]string{100: "mysqld", 101: "startup.sh", 102: "java"})
	cgroupRoot := setupMockCgroup(t, []int{100, 101, 102})

	resolver := NewPIDResolver(procRoot, cgroupRoot)
	result, err := resolver.ResolvePIDs("kubepods/pod-abc/container-xyz", []string{"mysqld", "java"})
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestPIDResolver_ResolvePIDs_NoMatch(t *testing.T) {
	procRoot := setupMockProc(t, map[int]string{100: "startup.sh"})
	cgroupRoot := setupMockCgroup(t, []int{100})

	resolver := NewPIDResolver(procRoot, cgroupRoot)
	result, err := resolver.ResolvePIDs("kubepods/pod-abc/container-xyz", []string{"mysqld"})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPIDResolver_ResolvePIDs_PIDDisappeared(t *testing.T) {
	procRoot := setupMockProc(t, map[int]string{100: "mysqld"})
	cgroupRoot := setupMockCgroup(t, []int{100, 999})

	resolver := NewPIDResolver(procRoot, cgroupRoot)
	result, err := resolver.ResolvePIDs("kubepods/pod-abc/container-xyz", []string{"mysqld"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "mysqld", result[0].Name)
}

func TestPIDResolver_ReadRSSKB(t *testing.T) {
	procRoot := setupMockProc(t, map[int]string{100: "mysqld"})
	resolver := NewPIDResolver(procRoot, "")
	rss, err := resolver.ReadRSSKB(100)
	require.NoError(t, err)
	assert.Equal(t, uint64(1048576), rss)
}

func TestBuildCgroupRelPath(t *testing.T) {
	tests := []struct {
		name     string
		podUID   string
		qosClass string
		expected string
	}{
		{"guaranteed", "abc-123", "Guaranteed", "memory/kubepods/podabc-123"},
		{"burstable", "def-456", "Burstable", "memory/kubepods/burstable/poddef-456"},
		{"besteffort", "ghi-789", "BestEffort", "memory/kubepods/besteffort/podghi-789"},
		{"empty qos defaults to besteffort", "xyz", "", "memory/kubepods/besteffort/podxyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCgroupRelPath(tt.podUID, tt.qosClass)
			if got != tt.expected {
				t.Errorf("BuildCgroupRelPath(%q, %q) = %q, want %q", tt.podUID, tt.qosClass, got, tt.expected)
			}
		})
	}
}
