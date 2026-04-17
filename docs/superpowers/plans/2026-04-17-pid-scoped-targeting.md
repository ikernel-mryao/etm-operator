# PID-Scoped Targeting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the node-wide `type=name` process matching by switching to `type=pid`, ensuring each etmemd project targets exactly one PID resolved from the Pod's cgroup — eliminating cross-Pod interference.

**Architecture:** The PID resolver already discovers PIDs from Pod cgroups (inherently Pod-scoped). Currently the resolved process name is passed to etmemd config as `type=name`, which etmemd resolves via `pgrep -x` (node-wide). The fix passes the actual PID as `type=pid`, making targeting exact. Each PID gets its own etmemd project. PID changes are handled naturally by the diff-based reconcile loop — stale PIDs disappear from cgroup → their projects get stopped; new PIDs appear → new projects get started.

**Tech Stack:** Go 1.21+, Kubernetes client-go, etmemd CLI, cgroup v1

**Key facts from etmemd source (etmemd_task.c):**
- `type=name` calls `/usr/bin/pgrep -x <value>` — node-wide, returns only FIRST match
- `type=pid` directly uses the PID string — exact, no ambiguity
- Both are the only valid task types (line 440: `strcmp(val, "pid") != 0 && strcmp(val, "name") != 0`)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/engine/interface.go` | Modify | Add `PID int` to `ProcessTarget` |
| `internal/engine/slide.go` | Modify | Change `type=name`/`value=<name>` → `type=pid`/`value=<pid>` |
| `internal/engine/slide_test.go` | Modify | Update tests for `type=pid` config format |
| `internal/agent/pod_watcher.go` | Modify | Add PID parameter to `ProjectNameForProcess` |
| `internal/agent/pod_watcher_test.go` | Modify | Update naming tests for PID parameter |
| `cmd/agent/main.go` | Modify | Remove name dedup, iterate PIDs, pass PID to ProcessTarget |
| `internal/agent/reconciler_test.go` | Modify | Add PID-change and cross-pod isolation tests |
| `scripts/verify-etmem-dataplane.sh` | Modify | Add scope verification checks |

---

### Task 1: Engine Layer — Add PID to ProcessTarget and Switch to type=pid

**Files:**
- Modify: `internal/engine/interface.go:8-10`
- Modify: `internal/engine/slide.go:39-44`
- Modify: `internal/engine/slide_test.go`

This task changes the config generation from `type=name`/`value=<processName>` to `type=pid`/`value=<PID>`. The `ProcessTarget` struct gains a `PID` field. This is the core fix — after this change, etmemd will target a specific PID instead of running `pgrep -x` node-wide.

- [ ] **Step 1: Write failing test for type=pid config**

In `internal/engine/slide_test.go`, add a test that verifies the generated config contains `type=pid` and `value=<PID>`:

```go
func TestSlideEngine_GenerateConfig_UsesPID(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("moderate")
	process := ProcessTarget{Name: "mysqld", PID: 12345}

	config := e.GenerateConfig("test-project", process, params)

	assert.Contains(t, config, "type=pid")
	assert.Contains(t, config, "value=12345")
	assert.NotContains(t, config, "type=name")
	assert.NotContains(t, config, "value=mysqld")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && PATH="/usr/local/go/bin:$PATH" go test ./internal/engine/ -run TestSlideEngine_GenerateConfig_UsesPID -v`
Expected: FAIL — PID field doesn't exist, config still uses `type=name`

- [ ] **Step 3: Add PID to ProcessTarget**

In `internal/engine/interface.go`, change `ProcessTarget`:

```go
// ProcessTarget represents a specific process to be managed by etmem.
// PID is the Linux process ID, resolved from the Pod's cgroup.
// Name is kept for logging and project naming (not used in etmemd config).
type ProcessTarget struct {
	Name string
	PID  int
}
```

- [ ] **Step 4: Change config generation to type=pid**

In `internal/engine/slide.go`, change lines 43-44 from:

```go
	fmt.Fprintf(&b, "type=name\n")
	fmt.Fprintf(&b, "value=%s\n", process.Name)
```

to:

```go
	fmt.Fprintf(&b, "type=pid\n")
	fmt.Fprintf(&b, "value=%d\n", process.PID)
```

Also add `"strconv"` to imports if needed (not needed since `%d` handles int formatting).

- [ ] **Step 5: Run test to verify it passes**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./internal/engine/ -run TestSlideEngine_GenerateConfig_UsesPID -v`
Expected: PASS

- [ ] **Step 6: Update existing tests to use PID field**

Update `TestSlideEngine_GenerateConfig_Moderate` and `TestSlideEngine_GenerateConfig_SingleProcess` to provide PID and check for `type=pid`:

In `TestSlideEngine_GenerateConfig_Moderate`:
- Change: `process := ProcessTarget{Name: "mysqld"}` → `process := ProcessTarget{Name: "mysqld", PID: 1234}`
- Change assertion: `assert.Contains(t, config, "value=mysqld")` → `assert.Contains(t, config, "value=1234")`
- Add: `assert.Contains(t, config, "type=pid")`

In `TestSlideEngine_GenerateConfig_SingleProcess`:
- Change: `process := ProcessTarget{Name: "java"}` → `process := ProcessTarget{Name: "java", PID: 5678}`
- Change assertion: `assert.Contains(t, config, "value=java")` → `assert.Contains(t, config, "value=5678")`
- Add: `assert.Contains(t, config, "type=pid")`

In `TestSlideEngine_WriteConfigFile`:
- Change: `process := ProcessTarget{Name: "mysqld"}` → `process := ProcessTarget{Name: "mysqld", PID: 9999}`

- [ ] **Step 7: Run all engine tests**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/engine/interface.go internal/engine/slide.go internal/engine/slide_test.go
git commit -m "feat: switch etmem config from type=name to type=pid

type=name uses pgrep -x which is node-wide and can cross-target
processes in other Pods. type=pid targets a specific PID resolved
from the Pod's cgroup, ensuring Pod-scoped isolation.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: PID-Based Project Naming

**Files:**
- Modify: `internal/agent/pod_watcher.go:56-63`
- Modify: `internal/agent/pod_watcher_test.go`

Project names must include PID to ensure uniqueness when multiple instances of the same process name exist in a Pod (e.g., 3 `worker` PIDs). Format: `{ns}-{pod}-{proc}-p{pid}`. The SHA256 truncation mechanism still applies for names >64 chars.

- [ ] **Step 1: Write failing test for PID in project name**

In `internal/agent/pod_watcher_test.go`, add:

```go
func TestProjectNameForProcess_IncludesPID(t *testing.T) {
	name := ProjectNameForProcess("default", "mypod", "worker", 12345)
	assert.Equal(t, "default-mypod-worker-p12345", name)
}

func TestProjectNameForProcess_DifferentPIDsDifferentNames(t *testing.T) {
	name1 := ProjectNameForProcess("default", "mypod", "worker", 100)
	name2 := ProjectNameForProcess("default", "mypod", "worker", 200)
	assert.NotEqual(t, name1, name2)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./internal/agent/ -run TestProjectNameForProcess_IncludesPID -v`
Expected: FAIL — function signature doesn't accept PID

- [ ] **Step 3: Update ProjectNameForProcess to include PID**

In `internal/agent/pod_watcher.go`, change the function:

```go
// ProjectNameForProcess generates the etmem project name for a specific process instance.
// Each PID needs its own project because etmemd rejects obj add for existing project names.
//
// Naming rule:
//   - Short names (≤64 chars): "{namespace}-{podName}-{processName}-p{pid}" — fully human-readable
//   - Long names (>64 chars):  "{55-char prefix}-{8-char SHA256 hex}" = 64 chars total
//     The hash is computed from the full untruncated name, so different inputs always
//     produce different outputs (with overwhelming probability).
//
// Properties:
//   - stable across reconcile loops (deterministic from inputs, PID doesn't change while process lives)
//   - unique per PID (PID in name or hash)
//   - unique across pods (namespace + podName in name or hash)
//   - max 64 characters guaranteed
func ProjectNameForProcess(namespace, podName, processName string, pid int) string {
	full := fmt.Sprintf("%s-%s-%s-p%d", namespace, podName, processName, pid)
	if len(full) <= 64 {
		return full
	}
	hash := sha256.Sum256([]byte(full))
	suffix := hex.EncodeToString(hash[:])[:8]
	return full[:55] + "-" + suffix
}
```

Add `"fmt"` to imports.

- [ ] **Step 4: Update existing tests for new signature**

Update all existing `ProjectNameForProcess` tests to pass a PID parameter (4th argument). Use PID=1000 as default for existing tests. For the truncation test, use a reasonable PID. For the collision test, use two different PIDs with same name.

Key changes to existing tests:
- `TestProjectNameForProcess_Basic`: `ProjectNameForProcess("default", "myapp-pod", "mysqld", 1000)` — expected: `"default-myapp-pod-mysqld-p1000"`
- `TestProjectNameForProcess_Truncation`: add PID, expected hash changes
- `TestProjectNameForProcess_Stable`: add PID=1000
- `TestProjectNameForProcess_UniqueProcesses`: keep different names + same PID
- `TestProjectNameForProcess_UniquePods`: keep same name + same PID but different pods
- `TestProjectNameForProcess_CollisionResistance`: add PID
- `TestProjectNameForProcess_ShortNameReadable`: add PID, check for "p1000" in result
- `TestProjectNameForProcess_ExactBoundary`: adjust to account for PID length

- [ ] **Step 5: Run all agent tests**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./internal/agent/ -v`
Expected: FAIL — build errors in main.go (callers not yet updated). That's OK, just verify pod_watcher_test.go tests pass in isolation:
Run: `PATH="/usr/local/go/bin:$PATH" go test ./internal/agent/ -run TestProjectNameForProcess -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/agent/pod_watcher.go internal/agent/pod_watcher_test.go
git commit -m "feat: add PID to project naming for per-PID uniqueness

ProjectNameForProcess now takes a PID parameter, producing names like
'default-mypod-worker-p12345'. This ensures uniqueness when multiple
instances of the same process name exist, and supports the type=pid
targeting model.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Agent Reconcile — PID-Based Iteration

**Files:**
- Modify: `cmd/agent/main.go:268-304`

This task updates the reconcile loop to:
1. Remove name-based deduplication — each PID gets its own project
2. Pass PID to `ProcessTarget` and `ProjectNameForProcess`
3. Use PID-based deduplication (a PID can only appear once)
4. Track PID in NodeState `ManagedProcess`

After this task, the full build must pass.

- [ ] **Step 1: Update the reconcile loop**

In `cmd/agent/main.go`, replace the process filtering and iteration block (lines ~268-304):

**Old code (name-based dedup):**
```go
			// Filter out infrastructure containers (pause/sandbox) and deduplicate.
			seenNames := make(map[string]bool)
			var processes []engine.ProcessTarget
			for _, pid := range pids {
				if isInfraProcess(pid.Name) {
					continue
				}
				if !seenNames[pid.Name] {
					processes = append(processes, engine.ProcessTarget{Name: pid.Name})
					seenNames[pid.Name] = true
				}
			}
			if len(processes) == 0 {
				logger.V(1).Info("No application processes after filtering infrastructure containers", "pod", pod.Name)
				continue
			}

			// One etmemd project per process: etmemd rejects obj add for existing
			// project names and only supports one [task] per config file.
			for _, proc := range processes {
				projectName := agent.ProjectNameForProcess(policy.Namespace, pod.Name, proc.Name)
				configContent := slideEngine.GenerateConfig(projectName, proc, params)

				desiredTasks[projectName] = desiredTaskInfo{
					Request: agent.TaskRequest{
						ProjectName:   projectName,
						ConfigContent: configContent,
					},
					PolicyRef: etmemv1.PolicyReference{
						Name:      policy.Name,
						Namespace: policy.Namespace,
					},
					PodName:     pod.Name,
					PodUID:      string(pod.UID),
					ProcessName: proc.Name,
				}
			}
```

**New code (PID-based, no name dedup):**
```go
			// Filter infrastructure containers (pause/sandbox) and deduplicate by PID.
			// Each PID gets its own etmemd project with type=pid targeting,
			// ensuring Pod-scoped isolation (no cross-Pod interference).
			seenPIDs := make(map[int]bool)
			var processes []engine.ProcessTarget
			for _, pid := range pids {
				if isInfraProcess(pid.Name) {
					continue
				}
				if !seenPIDs[pid.PID] {
					processes = append(processes, engine.ProcessTarget{Name: pid.Name, PID: pid.PID})
					seenPIDs[pid.PID] = true
				}
			}
			if len(processes) == 0 {
				logger.V(1).Info("No application processes after filtering infrastructure containers", "pod", pod.Name)
				continue
			}

			// One etmemd project per PID: type=pid targeting ensures exact process match.
			// Project name includes PID for uniqueness.
			for _, proc := range processes {
				projectName := agent.ProjectNameForProcess(policy.Namespace, pod.Name, proc.Name, proc.PID)
				configContent := slideEngine.GenerateConfig(projectName, proc, params)

				desiredTasks[projectName] = desiredTaskInfo{
					Request: agent.TaskRequest{
						ProjectName:   projectName,
						ConfigContent: configContent,
					},
					PolicyRef: etmemv1.PolicyReference{
						Name:      policy.Name,
						Namespace: policy.Namespace,
					},
					PodName:     pod.Name,
					PodUID:      string(pod.UID),
					ProcessName: proc.Name,
				}
			}
```

- [ ] **Step 2: Update NodeState ManagedProcess to include PID**

In `cmd/agent/main.go`, in the NodeState writing section (~line 339), update to include the actual PID:

Change:
```go
		Processes: []etmemv1.ManagedProcess{
			{Name: taskInfo.ProcessName},
		},
```

To:
```go
		Processes: []etmemv1.ManagedProcess{
			{Name: taskInfo.ProcessName, PID: taskInfo.PID},
		},
```

This requires adding `PID int` to `desiredTaskInfo`:

```go
type desiredTaskInfo struct {
	Request     agent.TaskRequest
	PolicyRef   etmemv1.PolicyReference
	PodName     string
	PodUID      string
	ProcessName string
	PID         int
}
```

And in the loop where `desiredTasks` is built, add: `PID: proc.PID,`

- [ ] **Step 3: Build and verify**

Run: `PATH="/usr/local/go/bin:$PATH" go build ./...`
Expected: PASS — all compilation errors resolved

- [ ] **Step 4: Run all tests**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./... -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/agent/main.go
git commit -m "feat: switch agent reconcile to PID-based iteration

Remove name-based deduplication. Each PID from the Pod's cgroup gets
its own etmemd project with type=pid targeting. This eliminates
node-wide cross-Pod interference from type=name/pgrep.

PID is now tracked in desiredTaskInfo and written to NodeState
ManagedProcess for observability.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 4: Tests — PID Lifecycle and Cross-Pod Isolation

**Files:**
- Modify: `internal/agent/reconciler_test.go`

Add tests for:
1. PID change (process restart) → old project stopped, new project started
2. Two processes same name different PIDs → separate projects
3. Config content reflects PID correctly

- [ ] **Step 1: Add PID-change lifecycle test**

In `internal/agent/reconciler_test.go`, add:

```go
func TestTaskManager_PIDChange_StopsOldStartsNew(t *testing.T) {
	mt := &mockTransport{responses: make(map[string]error)}
	tm := NewTaskManager(mt, t.TempDir())

	// Start with PID 100
	err := tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-pod-worker-p100",
		ConfigContent: "[project]\nname=default-pod-worker-p100\n[task]\ntype=pid\nvalue=100\n",
	})
	require.NoError(t, err)
	assert.True(t, tm.IsRunning("default-pod-worker-p100"))

	// Simulate PID change: stop old, start new
	err = tm.StopTask(context.Background(), "default-pod-worker-p100")
	require.NoError(t, err)
	assert.False(t, tm.IsRunning("default-pod-worker-p100"))

	err = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-pod-worker-p200",
		ConfigContent: "[project]\nname=default-pod-worker-p200\n[task]\ntype=pid\nvalue=200\n",
	})
	require.NoError(t, err)
	assert.True(t, tm.IsRunning("default-pod-worker-p200"))
	assert.False(t, tm.IsRunning("default-pod-worker-p100"))

	assert.Equal(t, 1, len(tm.RunningTasks()))
}
```

- [ ] **Step 2: Add same-name-different-PID test**

```go
func TestTaskManager_SameNameDifferentPIDs_SeparateProjects(t *testing.T) {
	mt := &mockTransport{responses: make(map[string]error)}
	tm := NewTaskManager(mt, t.TempDir())

	// Two worker processes with different PIDs
	err := tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-pod-worker-p100",
		ConfigContent: "[project]\nname=default-pod-worker-p100\n[task]\ntype=pid\nvalue=100\n",
	})
	require.NoError(t, err)

	err = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-pod-worker-p200",
		ConfigContent: "[project]\nname=default-pod-worker-p200\n[task]\ntype=pid\nvalue=200\n",
	})
	require.NoError(t, err)

	assert.True(t, tm.IsRunning("default-pod-worker-p100"))
	assert.True(t, tm.IsRunning("default-pod-worker-p200"))
	assert.Equal(t, 2, len(tm.RunningTasks()))

	// Stop one, other remains
	err = tm.StopTask(context.Background(), "default-pod-worker-p100")
	require.NoError(t, err)
	assert.False(t, tm.IsRunning("default-pod-worker-p100"))
	assert.True(t, tm.IsRunning("default-pod-worker-p200"))
}
```

- [ ] **Step 3: Add cross-pod isolation naming test**

```go
func TestProjectNaming_CrossPodIsolation(t *testing.T) {
	// Same process name in different pods → different project names
	nameA := ProjectNameForProcess("default", "podA", "mysqld", 100)
	nameB := ProjectNameForProcess("default", "podB", "mysqld", 200)
	assert.NotEqual(t, nameA, nameB)

	// Same process name, same pod name but different PIDs → different project names
	name1 := ProjectNameForProcess("default", "podA", "worker", 100)
	name2 := ProjectNameForProcess("default", "podA", "worker", 200)
	assert.NotEqual(t, name1, name2)
}
```

- [ ] **Step 4: Run all tests**

Run: `PATH="/usr/local/go/bin:$PATH" go test ./... -count=1 -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/reconciler_test.go
git commit -m "test: add PID lifecycle and cross-pod isolation tests

Tests cover: PID change → old stopped / new started, same process
name with different PIDs → separate projects, cross-pod naming
isolation.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 5: E2E Verification — Cross-Pod Isolation

**Prerequisites:** Tasks 1-4 complete and reviewed. Agent binary rebuilt and deployed.

This is the critical validation. Deploy two Pods with the same process name on the same node. Verify that enabling etmem on only one Pod does NOT affect the other.

- [ ] **Step 1: Build new agent binary**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
PATH="/usr/local/go/bin:$PATH" CGO_ENABLED=0 GOOS=linux go build -o bin/agent ./cmd/agent/
```

- [ ] **Step 2: Build agent container image v0.4.0-pid**

```bash
cat > /tmp/Dockerfile.agent-pid <<'EOF'
FROM etmem-agent:v0.2.0
COPY bin/agent /usr/local/bin/agent
EOF
sudo nerdctl --namespace k8s.io build -t etmem-agent:v0.4.0-pid -f /tmp/Dockerfile.agent-pid .
```

- [ ] **Step 3: Deploy new agent**

```bash
kubectl -n etmem-system set image daemonset/etmem-agent agent=etmem-agent:v0.4.0-pid
kubectl -n etmem-system rollout status daemonset/etmem-agent --timeout=60s
```

- [ ] **Step 4: Deploy two Pods with same process name**

Create Pod A (etmem enabled) and Pod B (etmem NOT enabled), both running a process called `memhog`:

```yaml
# /tmp/scope-test-a.yaml — etmem ENABLED
apiVersion: v1
kind: Pod
metadata:
  name: scope-test-a
  labels:
    etmem.openeuler.io/enable: "true"
  annotations:
    etmem.openeuler.io/profile: "aggressive"
spec:
  nodeName: <WORKER_NODE>
  containers:
  - name: memhog
    image: <REGISTRY>/multiproc-memhog:v1
    command: ["/memeat1"]
    args: ["150"]
    resources:
      requests:
        memory: "100Mi"
      limits:
        memory: "300Mi"
```

```yaml
# /tmp/scope-test-b.yaml — etmem NOT enabled (no label)
apiVersion: v1
kind: Pod
metadata:
  name: scope-test-b
spec:
  nodeName: <WORKER_NODE>
  containers:
  - name: memhog
    image: <REGISTRY>/multiproc-memhog:v1
    command: ["/memeat1"]
    args: ["150"]
    resources:
      requests:
        memory: "100Mi"
      limits:
        memory: "300Mi"
```

Note: Both use the same binary `/memeat1` so the process name (`memeat1`) is the same on both Pods.

- [ ] **Step 5: Verify scope isolation**

Wait 2-3 minutes for etmemd to scan, then check:

```bash
# Get PIDs from both pods
PID_A=$(cat /host/sys/fs/cgroup/memory/kubepods.slice/.../scope-test-a/.../cgroup.procs)
PID_B=$(cat /host/sys/fs/cgroup/memory/kubepods.slice/.../scope-test-b/.../cgroup.procs)

# Check VmSwap for both
grep VmSwap /host/proc/$PID_A/status  # Should show significant VmSwap
grep VmSwap /host/proc/$PID_B/status  # Should show 0 kB

# Check etmemd projects — should only have scope-test-a project
sudo etmem project show -s etmemd

# Check agent logs — should only reference scope-test-a
kubectl -n etmem-system logs daemonset/etmem-agent | grep scope-test
```

**Success criteria:**
- Pod A (enabled): VmSwap > 0, project exists
- Pod B (not enabled): VmSwap = 0, NO project
- etmemd configs contain `type=pid` and specific PIDs
- No config references Pod B's PID

- [ ] **Step 6: Verify both-enabled scenario**

Enable etmem on Pod B too:
```bash
kubectl label pod scope-test-b etmem.openeuler.io/enable=true
```

Wait 2-3 minutes, then verify:
- Both pods have VmSwap > 0
- Each pod has its own project with its own PID
- Projects do not reference each other's PIDs

- [ ] **Step 7: Collect artifacts**

```bash
mkdir -p artifacts/e2e-pid-scope
# Configs, logs, projects, VmSwap, etc.
```

- [ ] **Step 8: Commit artifacts**

```bash
git add artifacts/e2e-pid-scope/
git commit -m "feat: E2E verification of PID-scoped targeting

Verified: type=pid ensures Pod A's etmem project only targets Pod A's
PID. Pod B (same process name, same node) is not cross-targeted.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 6: Chinese Documentation Update

**Files:**
- Modify: `docs/本地数据面排查与验证说明.md`
- Modify: `docs/本地验证结果报告.md`
- Modify: `docs/本地最小功能验证说明.md`
- Modify: `docs/其他机器快速上手手册.md`
- Modify: `docs/真实目标环境验证说明.md`

Update all Chinese docs to reflect:
1. The `type=name` → `type=pid` change and why
2. Cross-pod isolation verification results
3. PID-based project naming
4. Updated verification steps

Each doc should clearly explain:
- What the old problem was (node-wide `pgrep -x`)
- How the fix works (PID from cgroup → `type=pid`)
- How to verify isolation (same-name processes, different pods)
- New project naming format

- [ ] **Step 1: Update each doc with PID-scope sections**
- [ ] **Step 2: Commit**

```bash
git add docs/
git commit -m "docs: 更新中文文档说明 type=pid 作用域修复

说明 type=name 节点范围匹配问题，type=pid 修复原理，
跨 Pod 隔离验证方法和结果。

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 7: Script Enhancement and GitHub Push

**Files:**
- Modify: `scripts/verify-etmem-dataplane.sh`
- Create: `scripts/verify-pid-scope-isolation.sh`

- [ ] **Step 1: Add scope verification to dataplane script**

Enhance `verify-etmem-dataplane.sh` to check that configs use `type=pid` (not `type=name`).

- [ ] **Step 2: Create scope isolation test script**

`scripts/verify-pid-scope-isolation.sh`:
- Deploy two pods with same process name
- Enable etmem on only one
- Wait and check VmSwap on both
- Output pass/fail verdict in Chinese

- [ ] **Step 3: Check GitHub push capability**

```bash
git remote -v
which gh && gh auth status
```

If no remote or auth available, document what's needed.

- [ ] **Step 4: Commit scripts**

```bash
git add scripts/
git commit -m "feat: add PID scope isolation verification script

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

- [ ] **Step 5: Tag release**

```bash
git tag -a v0.4.0-pid -m "v0.4.0-pid: PID-scoped targeting

Switches from type=name (node-wide pgrep) to type=pid (exact PID).
Eliminates cross-Pod interference on same node.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
