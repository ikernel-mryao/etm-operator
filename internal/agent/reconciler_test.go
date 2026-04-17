package agent

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

type mockTransport struct {
	objAdds    []string
	objDels    []string
	projStarts []string
	projStops  []string
	projShows  []string
	showOutput string
	err        error
}

func (m *mockTransport) ObjAdd(ctx context.Context, configPath string) error {
	m.objAdds = append(m.objAdds, configPath)
	return m.err
}
func (m *mockTransport) ObjDel(ctx context.Context, configPath string) error {
	m.objDels = append(m.objDels, configPath)
	return m.err
}
func (m *mockTransport) ProjectStart(ctx context.Context, name string) error {
	m.projStarts = append(m.projStarts, name)
	return m.err
}
func (m *mockTransport) ProjectStop(ctx context.Context, name string) error {
	m.projStops = append(m.projStops, name)
	return m.err
}
func (m *mockTransport) ProjectShow(ctx context.Context, name string) (string, error) {
	m.projShows = append(m.projShows, name)
	return m.showOutput, m.err
}

func TestTaskManager_StartTask(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	err := tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0",
		ConfigContent: "[project]\nname=default-mysql-0\n",
	})
	require.NoError(t, err)
	assert.Len(t, tr.objAdds, 1)
	assert.Len(t, tr.projStarts, 1)
	assert.True(t, tm.IsRunning("default-mysql-0"))
}

func TestTaskManager_StopTask(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	_ = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0",
		ConfigContent: "[project]\nname=default-mysql-0\n",
	})
	err := tm.StopTask(context.Background(), "default-mysql-0")
	require.NoError(t, err)
	assert.Len(t, tr.projStops, 1)
	assert.Len(t, tr.objDels, 1)
	assert.False(t, tm.IsRunning("default-mysql-0"))
}

func TestTaskManager_StopNonexistent(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	err := tm.StopTask(context.Background(), "nonexistent")
	assert.NoError(t, err)
}

// --- Multi-project lifecycle tests ---
// These tests verify TaskManager behavior when managing multiple per-process
// projects simultaneously, as required by the multi-process etmem model.

func TestTaskManager_MultiProject_StartMultiple(t *testing.T) {
	// Simulate a Pod with 3 application processes, each getting its own project.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	projects := []string{
		ProjectNameForProcess("default", "mysql-0", "mysqld", 1001),
		ProjectNameForProcess("default", "mysql-0", "exporter", 1002),
		ProjectNameForProcess("default", "mysql-0", "sidecar", 1003),
	}

	for _, name := range projects {
		err := tm.StartTask(ctx, TaskRequest{
			ProjectName:   name,
			ConfigContent: "[project]\nname=" + name + "\n",
		})
		require.NoError(t, err)
	}

	// All 3 must be running with separate transport calls.
	assert.Len(t, tr.objAdds, 3)
	assert.Len(t, tr.projStarts, 3)
	for _, name := range projects {
		assert.True(t, tm.IsRunning(name), "project %s should be running", name)
	}
	assert.Len(t, tm.RunningTasks(), 3)
}

func TestTaskManager_MultiProject_IdempotentReconcile(t *testing.T) {
	// The reconcile loop uses "if !IsRunning then StartTask".
	// Running the same desired set twice must not create duplicates.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	projects := []string{
		ProjectNameForProcess("ns", "pod-abc", "app", 2001),
		ProjectNameForProcess("ns", "pod-abc", "worker", 2002),
	}

	// First reconcile: start both projects.
	for _, name := range projects {
		if !tm.IsRunning(name) {
			require.NoError(t, tm.StartTask(ctx, TaskRequest{
				ProjectName:   name,
				ConfigContent: "[project]\nname=" + name + "\n",
			}))
		}
	}
	assert.Len(t, tr.objAdds, 2)
	assert.Len(t, tr.projStarts, 2)

	// Second reconcile: same desired set → no new transport calls.
	for _, name := range projects {
		if !tm.IsRunning(name) {
			require.NoError(t, tm.StartTask(ctx, TaskRequest{
				ProjectName:   name,
				ConfigContent: "[project]\nname=" + name + "\n",
			}))
		}
	}
	assert.Len(t, tr.objAdds, 2, "no duplicate obj adds on re-reconcile")
	assert.Len(t, tr.projStarts, 2, "no duplicate project starts on re-reconcile")
}

func TestTaskManager_MultiProject_CleanupOnProcessDisappear(t *testing.T) {
	// When a process disappears from a Pod, the reconcile diff loop calls
	// StopTask for projects no longer in the desired set.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	proj1 := ProjectNameForProcess("default", "app-0", "main", 3001)
	proj2 := ProjectNameForProcess("default", "app-0", "helper", 3002)
	proj3 := ProjectNameForProcess("default", "app-0", "logger", 3003)

	// Start all 3.
	for _, name := range []string{proj1, proj2, proj3} {
		require.NoError(t, tm.StartTask(ctx, TaskRequest{
			ProjectName:   name,
			ConfigContent: "[project]\nname=" + name + "\n",
		}))
	}
	assert.Len(t, tm.RunningTasks(), 3)

	// New desired set: "helper" process disappeared.
	desired := map[string]bool{proj1: true, proj3: true}

	// Diff loop: stop tasks not in desired (same pattern as agentReconcile).
	for _, name := range tm.RunningTasks() {
		if !desired[name] {
			require.NoError(t, tm.StopTask(ctx, name))
		}
	}

	assert.True(t, tm.IsRunning(proj1), "main should still run")
	assert.False(t, tm.IsRunning(proj2), "helper should be stopped")
	assert.True(t, tm.IsRunning(proj3), "logger should still run")
	assert.Len(t, tm.RunningTasks(), 2)
	assert.Contains(t, tr.projStops, proj2)
}

func TestTaskManager_MultiProject_BootstrapRecovery(t *testing.T) {
	// After Agent restart, BootstrapFromNodeState must recover per-process
	// projects using their deterministic names.
	tmpDir := t.TempDir()
	tr := &mockTransport{}
	tm := NewTaskManager(tr, tmpDir)

	// Derive project names the same way agentReconcile does.
	proj1 := ProjectNameForProcess("prod", "db-7f8a", "postgres", 4001)
	proj2 := ProjectNameForProcess("prod", "db-7f8a", "pgbouncer", 4002)

	// Simulate config files left from before restart.
	for _, name := range []string{proj1, proj2} {
		path := tmpDir + "/" + name + ".conf"
		require.NoError(t, os.WriteFile(path, []byte("[project]\nname="+name+"\n"), 0600))
	}

	// Bootstrap from NodeState (as reported before crash).
	tasks := []etmemv1.NodeTask{
		{ProjectName: proj1, State: "running"},
		{ProjectName: proj2, State: "running"},
		{ProjectName: "stale-project", State: "running"}, // no config file → skipped
	}
	recovered := tm.BootstrapFromNodeState(tasks)

	assert.Equal(t, 2, recovered)
	assert.True(t, tm.IsRunning(proj1))
	assert.True(t, tm.IsRunning(proj2))
	assert.False(t, tm.IsRunning("stale-project"))
}

func TestTaskManager_MultiProject_StopAll(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	names := []string{
		ProjectNameForProcess("ns", "pod", "a", 5001),
		ProjectNameForProcess("ns", "pod", "b", 5002),
		ProjectNameForProcess("ns", "pod", "c", 5003),
	}
	for _, name := range names {
		require.NoError(t, tm.StartTask(ctx, TaskRequest{
			ProjectName:   name,
			ConfigContent: "[project]\nname=" + name + "\n",
		}))
	}
	assert.Len(t, tm.RunningTasks(), 3)

	tm.StopAll(ctx)
	assert.Len(t, tm.RunningTasks(), 0)
	assert.Len(t, tr.projStops, 3)
}

func TestTaskManager_MultiProject_NameDerivedIdentity(t *testing.T) {
	// Verify that project naming through ProjectNameForProcess produces
	// correct, stable identities that survive the full lifecycle.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	// Derive names exactly as agentReconcile does.
	ns, pod := "monitoring", "prometheus-k8s-0"
	proc1Name := ProjectNameForProcess(ns, pod, "prometheus", 6001)
	proc2Name := ProjectNameForProcess(ns, pod, "thanos", 6002)

	// Names must be different.
	assert.NotEqual(t, proc1Name, proc2Name)

	// Start both.
	for _, name := range []string{proc1Name, proc2Name} {
		require.NoError(t, tm.StartTask(ctx, TaskRequest{
			ProjectName:   name,
			ConfigContent: "[project]\nname=" + name + "\n",
		}))
	}

	// Re-derive names (simulating next reconcile) — must match.
	assert.Equal(t, proc1Name, ProjectNameForProcess(ns, pod, "prometheus", 6001))
	assert.Equal(t, proc2Name, ProjectNameForProcess(ns, pod, "thanos", 6002))

	// IsRunning with re-derived names must still work.
	assert.True(t, tm.IsRunning(ProjectNameForProcess(ns, pod, "prometheus", 6001)))
	assert.True(t, tm.IsRunning(ProjectNameForProcess(ns, pod, "thanos", 6002)))

	// Stop one using re-derived name.
	require.NoError(t, tm.StopTask(ctx, ProjectNameForProcess(ns, pod, "thanos", 6002)))
	assert.True(t, tm.IsRunning(proc1Name))
	assert.False(t, tm.IsRunning(proc2Name))
}

func TestTaskManager_PIDChange_ProjectReplacement(t *testing.T) {
	// When a process restarts, its PID changes.
	// Old project (old PID) should be stopped, new project (new PID) started.
	// This simulates the reconcile diff behavior.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	// First reconcile: process "mysqld" has PID 1000
	oldProj := ProjectNameForProcess("default", "mysql-0", "mysqld", 1000)
	require.NoError(t, tm.StartTask(ctx, TaskRequest{
		ProjectName:   oldProj,
		ConfigContent: "[project]\nname=" + oldProj + "\n",
	}))
	assert.True(t, tm.IsRunning(oldProj))

	// Process restarts → new PID 2000
	newProj := ProjectNameForProcess("default", "mysql-0", "mysqld", 2000)
	assert.NotEqual(t, oldProj, newProj, "different PIDs must produce different project names")

	// Reconcile diff: old project not in desired set → stop it
	desired := map[string]bool{newProj: true}
	for _, name := range tm.RunningTasks() {
		if !desired[name] {
			require.NoError(t, tm.StopTask(ctx, name))
		}
	}
	// Start new project
	require.NoError(t, tm.StartTask(ctx, TaskRequest{
		ProjectName:   newProj,
		ConfigContent: "[project]\nname=" + newProj + "\n",
	}))

	assert.False(t, tm.IsRunning(oldProj), "old PID project should be stopped")
	assert.True(t, tm.IsRunning(newProj), "new PID project should be running")
	assert.Contains(t, tr.projStops, oldProj)
	assert.Contains(t, tr.projStarts, newProj)
}

func TestTaskManager_CrossPodIsolation(t *testing.T) {
	// Two pods on the same node, both running a process named "worker".
	// Each pod's process has a different PID → different project names → no collision.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	podA_proj := ProjectNameForProcess("default", "app-pod-a", "worker", 100)
	podB_proj := ProjectNameForProcess("default", "app-pod-b", "worker", 200)

	assert.NotEqual(t, podA_proj, podB_proj, "same process name in different pods must have different project names")

	// Start both
	require.NoError(t, tm.StartTask(ctx, TaskRequest{
		ProjectName:   podA_proj,
		ConfigContent: "[project]\nname=" + podA_proj + "\n",
	}))
	require.NoError(t, tm.StartTask(ctx, TaskRequest{
		ProjectName:   podB_proj,
		ConfigContent: "[project]\nname=" + podB_proj + "\n",
	}))

	assert.True(t, tm.IsRunning(podA_proj))
	assert.True(t, tm.IsRunning(podB_proj))
	assert.Len(t, tm.RunningTasks(), 2)

	// Stop Pod A's project without affecting Pod B
	require.NoError(t, tm.StopTask(ctx, podA_proj))
	assert.False(t, tm.IsRunning(podA_proj))
	assert.True(t, tm.IsRunning(podB_proj), "Pod B's project must not be affected")
}

func TestTaskManager_SamePodSameNameDifferentPIDs(t *testing.T) {
	// A pod may have multiple instances of same-name process (e.g., worker pool).
	// With type=pid, each instance gets its own project.
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	ctx := context.Background()

	proj1 := ProjectNameForProcess("default", "pool-pod", "worker", 3001)
	proj2 := ProjectNameForProcess("default", "pool-pod", "worker", 3002)
	proj3 := ProjectNameForProcess("default", "pool-pod", "worker", 3003)

	assert.NotEqual(t, proj1, proj2)
	assert.NotEqual(t, proj2, proj3)
	assert.NotEqual(t, proj1, proj3)

	for _, name := range []string{proj1, proj2, proj3} {
		require.NoError(t, tm.StartTask(ctx, TaskRequest{
			ProjectName:   name,
			ConfigContent: "[project]\nname=" + name + "\n",
		}))
	}

	assert.Len(t, tm.RunningTasks(), 3)
	assert.Len(t, tr.objAdds, 3)
	assert.Len(t, tr.projStarts, 3)
}
