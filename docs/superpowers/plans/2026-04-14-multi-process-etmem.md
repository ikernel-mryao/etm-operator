# Multi-Process etmem Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support multiple application processes per Pod in etmem memory swap, with one etmemd project per process (not per Pod).

**Architecture:** etmemd only supports one `[task]` section per config file and rejects `obj add` when a project name already exists. The correct multi-process model is: one etmemd project per process, each with its own config file. Multiple projects run simultaneously. The Agent's reconcile loop builds N `desiredTaskInfo` entries per Pod (one per discovered application process), and TaskManager tracks each independently.

**Tech Stack:** Go, etmem CLI, Kubernetes controller-runtime

---

## Background: etmemd Limitation (verified live)

```
# First obj add succeeds:
etmem obj add -f config1.conf -s etmemd_socket   → OK

# Second obj add with same project name fails:
etmem obj add -f config2.conf -s etmemd_socket   → "error: project has been existed"

# But separate project names work fine:
# config1.conf: [project] name=pod-proc0
# config2.conf: [project] name=pod-proc1
# Both load and start successfully.
```

**Current behavior:** Agent discovers all processes but truncates to `processes[:1]` at line 287 of `cmd/agent/main.go`. Only one process per Pod gets swap management.

**Target behavior:** Agent creates separate etmemd projects for each application process in a Pod. All processes get swap management.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/engine/interface.go` | Modify | Change Engine interface to accept single `ProcessTarget` |
| `internal/engine/slide.go` | Modify | Update `GenerateConfig`/`WriteConfigFile` for single process |
| `internal/engine/slide_test.go` | Modify | Update tests for single-process config, add multi-project test |
| `internal/agent/pod_watcher.go` | Modify | Add `ProjectNameForProcess()` function |
| `internal/agent/pod_watcher_test.go` | Modify | Add test for new naming function |
| `cmd/agent/main.go` | Modify | Remove `[:1]` workaround, generate N projects per Pod |
| `internal/agent/reconciler_test.go` | Modify | Add multi-project start/stop test |

**Files NOT changed:**
- `internal/agent/reconciler.go` — TaskManager already works per-project, no changes needed
- `internal/transport/*` — Transport interface already per-project, no changes needed
- `api/v1alpha1/*` — NodeTask already has `ProjectName` field, multiple NodeTasks per Pod is valid
- `internal/engine/profiles.go` — No changes needed

---

### Task 1: Change Engine interface to single ProcessTarget

**Files:**
- Modify: `internal/engine/interface.go:12-16`
- Modify: `internal/engine/slide.go:1-11` (comment), `22-55` (GenerateConfig), `57-68` (WriteConfigFile)
- Modify: `internal/engine/slide_test.go:13-68` (all tests)

The core change: `GenerateConfig` and `WriteConfigFile` accept a single `ProcessTarget` instead of `[]ProcessTarget`. This enforces the etmemd "one task per config" constraint at the type level.

- [ ] **Step 1: Write the failing test — single process config generation**

Replace the existing test file `internal/engine/slide_test.go` with updated tests. The key change is:
- `TestSlideEngine_GenerateConfig_Moderate`: passes single `ProcessTarget` (not slice)
- `TestSlideEngine_GenerateConfig_SingleProcess`: replaces `MultipleProcesses` test, asserts exactly 1 `[task]` section
- `TestSlideEngine_WriteConfigFile`: passes single `ProcessTarget`
- `TestSlideEngine_ApplyOverrides`: unchanged (doesn't touch config generation)

Edit `internal/engine/slide_test.go`:

```go
package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func TestSlideEngine_GenerateConfig_Moderate(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("moderate")
	process := ProcessTarget{Name: "mysqld"}

	config := e.GenerateConfig("test-project", process, params)

	assert.Contains(t, config, "[project]")
	assert.Contains(t, config, "name=test-project")
	assert.Contains(t, config, "loop=1")
	assert.Contains(t, config, "interval=1")
	assert.Contains(t, config, "sleep=1")
	assert.Contains(t, config, "sysmem_threshold=90")
	assert.Contains(t, config, "[engine]")
	assert.Contains(t, config, "name=slide")
	assert.Contains(t, config, "[task]")
	assert.Contains(t, config, "value=mysqld")
	assert.Contains(t, config, "T=1")
	assert.Contains(t, config, "swap_flag=no")
}

func TestSlideEngine_GenerateConfig_SingleProcess(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("conservative")
	process := ProcessTarget{Name: "java"}

	config := e.GenerateConfig("single-proj", process, params)

	taskCount := strings.Count(config, "[task]")
	assert.Equal(t, 1, taskCount, "etmemd only supports one [task] per config")
	assert.Contains(t, config, "value=java")
	assert.Contains(t, config, "name=single-proj_task_0")
}

func TestSlideEngine_ApplyOverrides(t *testing.T) {
	params, _ := GetProfile("moderate")
	loop := 5
	interval := 10
	overrides := &etmemv1.SlideOverrides{Loop: &loop, Interval: &interval}

	result := ApplyOverrides(params, overrides)
	assert.Equal(t, 5, result.Loop)
	assert.Equal(t, 10, result.Interval)
	assert.Equal(t, 1, result.Sleep) // unchanged
}

func TestSlideEngine_WriteConfigFile(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("moderate")
	process := ProcessTarget{Name: "mysqld"}

	path, err := e.WriteConfigFile(t.TempDir(), "test-proj", process, params)
	require.NoError(t, err)
	assert.Contains(t, path, "test-proj")
	assert.True(t, strings.HasSuffix(path, ".conf"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./internal/engine/ -v -run 'TestSlideEngine'`

Expected: FAIL — `GenerateConfig` and `WriteConfigFile` still expect `[]ProcessTarget`, not `ProcessTarget`.

- [ ] **Step 3: Update Engine interface and SlideEngine implementation**

Edit `internal/engine/interface.go` lines 14-15 — change `[]ProcessTarget` to `ProcessTarget`:

```go
// Engine defines the interface for etmem engine implementations.
type Engine interface {
	GenerateConfig(projectName string, process ProcessTarget, params SlideParams) string
	WriteConfigFile(dir, projectName string, process ProcessTarget, params SlideParams) (string, error)
}
```

Edit `internal/engine/slide.go` — replace entire file content:

```go
// SlideEngine 负责生成 etmem slide 引擎的配置文件。
// 配置文件格式（INI 风格）：
//   [project] 定义全局扫描参数（loop、interval、内存阈值等）
//   [engine]  声明引擎类型为 slide
//   [task]    一个进程对应一个 task 块，定义换出策略（T、swap_threshold 等）
// 参数映射：Kubernetes API SlideParams → etmem CLI 配置字段
//
// 重要限制：etmemd 只支持每个配置文件一个 [task] 段，且同名 project 的
// 第二次 obj add 会返回 "project has been existed" 错误。
// 因此每个进程必须对应一个独立的 project 和独立的配置文件。
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SlideEngine struct{}

func (e *SlideEngine) GenerateConfig(projectName string, process ProcessTarget, params SlideParams) string {
	var b strings.Builder

	fmt.Fprintf(&b, "[project]\n")
	fmt.Fprintf(&b, "name=%s\n", projectName)
	fmt.Fprintf(&b, "scan_type=page\n")
	fmt.Fprintf(&b, "loop=%d\n", params.Loop)
	fmt.Fprintf(&b, "interval=%d\n", params.Interval)
	fmt.Fprintf(&b, "sleep=%d\n", params.Sleep)
	fmt.Fprintf(&b, "sysmem_threshold=%d\n", params.SysMemThreshold)
	fmt.Fprintf(&b, "swapcache_high_wmark=%d\n", params.SwapCacheHighWmark)
	fmt.Fprintf(&b, "swapcache_low_wmark=%d\n", params.SwapCacheLowWmark)

	fmt.Fprintf(&b, "\n[engine]\n")
	fmt.Fprintf(&b, "name=slide\n")
	fmt.Fprintf(&b, "project=%s\n", projectName)

	fmt.Fprintf(&b, "\n[task]\n")
	fmt.Fprintf(&b, "project=%s\n", projectName)
	fmt.Fprintf(&b, "engine=slide\n")
	fmt.Fprintf(&b, "name=%s_task_0\n", projectName)
	fmt.Fprintf(&b, "type=name\n")
	fmt.Fprintf(&b, "value=%s\n", process.Name)
	fmt.Fprintf(&b, "T=%d\n", params.T)
	fmt.Fprintf(&b, "max_threads=%d\n", params.MaxThreads)
	if params.SwapThreshold != "" {
		fmt.Fprintf(&b, "swap_threshold=%s\n", params.SwapThreshold)
	}
	fmt.Fprintf(&b, "swap_flag=%s\n", params.SwapFlag)

	return b.String()
}

func (e *SlideEngine) WriteConfigFile(dir, projectName string, process ProcessTarget, params SlideParams) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	content := e.GenerateConfig(projectName, process, params)
	path := filepath.Join(dir, projectName+".conf")
	// etmemd 安全要求：配置文件权限必须为 600 或 400
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("write config file: %w", err)
	}
	return path, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./internal/engine/ -v -run 'TestSlideEngine'`

Expected: All 4 tests PASS.

- [ ] **Step 5: Verify full test suite still compiles**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go build ./...`

Expected: FAIL — `cmd/agent/main.go` still calls `GenerateConfig` with `[]ProcessTarget`. This is expected and will be fixed in Task 3.

- [ ] **Step 6: Commit engine changes**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add internal/engine/interface.go internal/engine/slide.go internal/engine/slide_test.go
git commit -m "refactor: change Engine interface to single ProcessTarget per config

etmemd only supports one [task] per INI config file and rejects obj add
when a project name already exists. Change GenerateConfig/WriteConfigFile
to accept a single ProcessTarget instead of a slice, enforcing this
constraint at the type level.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Add ProjectNameForProcess helper

**Files:**
- Modify: `internal/agent/pod_watcher.go:44-51`
- Modify: `internal/agent/pod_watcher_test.go` (add new test)

The current `ProjectName(namespace, podName)` generates one name per Pod. We need `ProjectNameForProcess(namespace, podName, processName)` that generates a unique name per process.

- [ ] **Step 1: Write the failing test**

Add to `internal/agent/pod_watcher_test.go`:

```go
func TestProjectNameForProcess(t *testing.T) {
	name := ProjectNameForProcess("default", "mysql-0", "mysqld")
	assert.Equal(t, "default-mysql-0-mysqld", name)
}

func TestProjectNameForProcess_Truncation(t *testing.T) {
	long := strings.Repeat("a", 60)
	name := ProjectNameForProcess(long, "pod-name", "proc")
	assert.LessOrEqual(t, len(name), 64)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./internal/agent/ -v -run 'TestProjectNameForProcess'`

Expected: FAIL — `ProjectNameForProcess` undefined.

- [ ] **Step 3: Implement ProjectNameForProcess**

Add to `internal/agent/pod_watcher.go` after the existing `ProjectName` function:

```go
// ProjectNameForProcess generates the etmem project name for a specific process in a pod.
// Each process needs its own project because etmemd rejects obj add for existing project names.
func ProjectNameForProcess(namespace, podName, processName string) string {
	name := namespace + "-" + podName + "-" + processName
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./internal/agent/ -v -run 'TestProjectNameForProcess'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add internal/agent/pod_watcher.go internal/agent/pod_watcher_test.go
git commit -m "feat: add ProjectNameForProcess for per-process project naming

etmemd requires unique project names and rejects duplicate obj add.
Multi-process Pods need one project per process, so project names
now include the process name: {namespace}-{podName}-{processName}.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Update Agent reconcile for multi-process support

**Files:**
- Modify: `cmd/agent/main.go:152-157` (desiredTaskInfo struct), `266-303` (process loop)

This is the core behavior change. Instead of one `desiredTaskInfo` per Pod, we create one per process per Pod. The diff/start/stop logic in lines 307-325 already works per-project and needs no changes.

- [ ] **Step 1: Write the failing test — verify build compiles**

After Task 1 changed the Engine interface, `cmd/agent/main.go` won't compile. First verify the build fails:

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go build ./cmd/agent/ 2>&1 | head -5`

Expected: Compilation error on `GenerateConfig` call (wrong argument type).

- [ ] **Step 2: Update the desiredTaskInfo struct**

Edit `cmd/agent/main.go` — add `ProcessName` field to `desiredTaskInfo`:

Replace lines 152-157:

```go
type desiredTaskInfo struct {
	Request     agent.TaskRequest
	PolicyRef   etmemv1.PolicyReference
	PodName     string
	PodUID      string
	ProcessName string
}
```

- [ ] **Step 3: Rewrite the process discovery and project generation loop**

Replace the block from line 266 (`// Filter out infrastructure containers`) through line 303 (closing brace of `desiredTasks[projectName]` assignment) with the new multi-process logic:

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

- [ ] **Step 4: Update NodeState reporting to include process name**

In the NodeState writing block (around line 331-340), update to include process info:

Replace the nodeTasks construction:

```go
	nodeTasks := make([]etmemv1.NodeTask, 0, len(desiredTasks))
	for projectName, taskInfo := range desiredTasks {
		nodeTasks = append(nodeTasks, etmemv1.NodeTask{
			ProjectName: projectName,
			PolicyRef:   taskInfo.PolicyRef,
			PodName:     taskInfo.PodName,
			PodUID:      taskInfo.PodUID,
			Processes: []etmemv1.ManagedProcess{
				{Name: taskInfo.ProcessName},
			},
			State: "running",
		})
	}
```

- [ ] **Step 5: Update ManagedPodsTotal metric to count unique Pods**

The current `ManagedPodsTotal.Set(float64(len(desiredTasks)))` would now count projects (processes), not Pods. Fix to count unique Pods:

Replace the metrics line:

```go
	// Count unique Pods (not projects) for the metric
	uniquePods := make(map[string]bool)
	for _, taskInfo := range desiredTasks {
		uniquePods[taskInfo.PodName] = true
	}
	agent.ManagedPodsTotal.Set(float64(len(uniquePods)))
```

- [ ] **Step 6: Verify build succeeds**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go build ./...`

Expected: SUCCESS — no compilation errors.

- [ ] **Step 7: Run full test suite**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./... 2>&1`

Expected: All tests pass. (Some tests may need updating if they reference the old `GenerateConfig` signature — if so, fix them before committing.)

- [ ] **Step 8: Commit**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add cmd/agent/main.go
git commit -m "feat: support multiple processes per Pod with per-process projects

Remove the processes[:1] workaround. For each application process in a
Pod, create a separate etmemd project with its own config file. This
correctly maps to etmemd's model where each project supports exactly
one task.

Project naming: {namespace}-{podName}-{processName}
NodeState: one NodeTask entry per process (not per Pod)
Metrics: ManagedPodsTotal still counts unique Pods

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 4: Update reconciler tests for multi-project behavior

**Files:**
- Modify: `internal/agent/reconciler_test.go`

Add tests that verify TaskManager handles multiple projects for the same Pod correctly (start multiple, stop all when Pod is removed).

- [ ] **Step 1: Write the new tests**

Add to `internal/agent/reconciler_test.go`:

```go
func TestTaskManager_MultipleProjectsPerPod(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())

	// Start two projects for the same Pod (different processes)
	err := tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0-mysqld",
		ConfigContent: "[project]\nname=default-mysql-0-mysqld\n",
	})
	require.NoError(t, err)

	err = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0-java",
		ConfigContent: "[project]\nname=default-mysql-0-java\n",
	})
	require.NoError(t, err)

	assert.True(t, tm.IsRunning("default-mysql-0-mysqld"))
	assert.True(t, tm.IsRunning("default-mysql-0-java"))
	assert.Len(t, tr.objAdds, 2)
	assert.Len(t, tr.projStarts, 2)
	assert.Len(t, tm.RunningTasks(), 2)
}

func TestTaskManager_StopMultipleProjectsIndependently(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())

	_ = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0-mysqld",
		ConfigContent: "[project]\nname=default-mysql-0-mysqld\n",
	})
	_ = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0-java",
		ConfigContent: "[project]\nname=default-mysql-0-java\n",
	})

	// Stop one project
	err := tm.StopTask(context.Background(), "default-mysql-0-mysqld")
	require.NoError(t, err)

	assert.False(t, tm.IsRunning("default-mysql-0-mysqld"))
	assert.True(t, tm.IsRunning("default-mysql-0-java"))
	assert.Len(t, tr.projStops, 1)
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd /home/ygz/work/etmem-workspace/etmem-operator && go test ./internal/agent/ -v -run 'TestTaskManager_Multiple'`

Expected: PASS (TaskManager already supports multiple projects — these tests confirm that).

- [ ] **Step 3: Commit**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add internal/agent/reconciler_test.go
git commit -m "test: add multi-project TaskManager tests

Verify that TaskManager correctly handles multiple concurrent projects
(as needed for multi-process Pods) and can stop them independently.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 5: E2E dataplane verification with multi-process Pod

**Files:**
- Create: `test/e2e/multi-process-pod.yaml`
- No code changes — this is a live verification task

This task must be executed on the real cluster (cp0 node with etmemd running). The goal is to prove that multiple processes in a single Pod each get their own etmemd project and VmSwap grows for each.

- [ ] **Step 1: Build and deploy updated Agent image**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
make build
# Build agent image with the new multi-process logic
docker build -t etmem-agent:v0.2.0-multi -f Dockerfile.agent .
# Import into containerd for the local cluster
ctr -n k8s.io images import <(docker save etmem-agent:v0.2.0-multi)
# Or use nerdctl directly:
# nerdctl build -t etmem-agent:v0.2.0-multi -f Dockerfile.agent .
```

Update the running Agent DaemonSet to use the new image:

```bash
kubectl set image daemonset/etmem-agent agent=etmem-agent:v0.2.0-multi
kubectl rollout status daemonset/etmem-agent --timeout=60s
```

- [ ] **Step 2: Create multi-process test Pod**

Create `test/e2e/multi-process-pod.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: multi-proc-test
  namespace: default
  labels:
    app: multi-proc-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multi-proc-test
  template:
    metadata:
      labels:
        app: multi-proc-test
        etmem.openeuler.io/enable: "true"
      annotations:
        etmem.openeuler.io/profile: "aggressive"
    spec:
      containers:
      - name: memhog1
        image: busybox:latest
        command:
        - sh
        - -c
        - |
          # Allocate ~150MB and hold it
          dd if=/dev/urandom bs=1M count=150 of=/dev/shm/data1 2>/dev/null
          tail -f /dev/null
        resources:
          requests:
            memory: "200Mi"
          limits:
            memory: "300Mi"
      - name: memhog2
        image: busybox:latest
        command:
        - sh
        - -c
        - |
          # Allocate ~150MB and hold it
          dd if=/dev/urandom bs=1M count=150 of=/dev/shm/data2 2>/dev/null
          tail -f /dev/null
        resources:
          requests:
            memory: "200Mi"
          limits:
            memory: "300Mi"
```

Apply it:

```bash
kubectl apply -f test/e2e/multi-process-pod.yaml
kubectl wait --for=condition=Available deployment/multi-proc-test --timeout=120s
```

- [ ] **Step 3: Verify Operator creates auto-policy for new Pod**

```bash
# Check auto-generated policy exists
kubectl get etmempolicy -A
# Expected: etmem-auto policy in default namespace

# Check Agent logs for multi-process discovery
kubectl logs daemonset/etmem-agent --tail=50 | grep -E 'Matched pod|Task started|project'
# Expected: Two "Task started" log lines for the multi-proc-test Pod
# - one for each process (e.g., "sh" or container main process)
```

- [ ] **Step 4: Verify etmemd has two active projects**

```bash
# List all etmemd projects
etmem project show -s etmemd_socket
# Expected: Two projects like:
#   default-multi-proc-test-xxx-<proc1>
#   default-multi-proc-test-xxx-<proc2>

# Check config files
ls -la /etc/etmem/
# Expected: Two .conf files, one per project
```

- [ ] **Step 5: Verify VmSwap for each process**

```bash
# Find the Pod name
POD=$(kubectl get pod -l app=multi-proc-test -o jsonpath='{.items[0].metadata.name}')

# Find PIDs of container processes
# Method: check etmemd logs or /proc for processes matching the Pod's cgroup
# Look for VmSwap in each process's /proc/<PID>/status
for PID in $(pgrep -f "tail -f /dev/null"); do
  echo "=== PID $PID ==="
  grep -E 'VmRSS|VmSwap|Name' /proc/$PID/status
done
# Expected: VmSwap > 0 for both container main processes
```

- [ ] **Step 6: Record results**

Collect into `artifacts/multi-process-verification/`:

```bash
mkdir -p artifacts/multi-process-verification
kubectl get etmempolicy -A -o yaml > artifacts/multi-process-verification/policies.yaml
kubectl logs daemonset/etmem-agent --tail=200 > artifacts/multi-process-verification/agent-logs.txt
etmem project show -s etmemd_socket > artifacts/multi-process-verification/etmem-projects.txt 2>&1 || true
ls -la /etc/etmem/ > artifacts/multi-process-verification/config-files.txt
# Per-process VmSwap evidence
for PID in $(pgrep -f "tail -f /dev/null"); do
  grep -E 'VmRSS|VmSwap|Name' /proc/$PID/status >> artifacts/multi-process-verification/vmswap-evidence.txt
done
```

- [ ] **Step 7: Clean up test Pod (optional — keep if continuing validation)**

```bash
kubectl delete -f test/e2e/multi-process-pod.yaml
```

---

### Task 6: Update Chinese documentation

**Files:**
- Modify: `docs/本地数据面排查与验证说明.md`
- Modify: `docs/本地验证结果报告.md`
- Modify: `docs/其他机器快速上手手册.md`
- Modify: `docs/真实目标环境验证说明.md`

- [ ] **Step 1: Add multi-process section to 本地数据面排查与验证说明.md**

Append a new section (section 4 or equivalent) titled **"多进程数据面验证"** that explains:

1. **etmemd 的多进程限制：** 为什么不能在 INI 里写多个 `[task]`
   - etmemd 的 INI 解析器只保留最后一个 `[task]` 段
   - 对同一 project name 再次执行 `obj add` 会返回 "project has been existed"
2. **正确的多进程模型：** 一个进程对应一个 project
   - 每个进程有独立的配置文件、独立的 project name
   - 多个 project 可以同时运行
3. **Agent 实现方式：**
   - project 命名：`{namespace}-{podName}-{processName}`
   - 每个进程独立 `obj add` + `project start`
   - Pod 删除时，所有关联 project 自动 stop
4. **多进程验证结果表** (from Task 5 artifacts)

- [ ] **Step 2: Update 本地验证结果报告.md**

Add a new section titled **"多进程验证结果"** with:

- 测试 Pod 配置（双容器、每容器 ~150MB）
- 每个进程的 VmRSS / VmSwap 变化
- 单进程 vs 多进程对比结论
- etmemd project 列表截图/输出

- [ ] **Step 3: Update 其他机器快速上手手册.md**

Add a subsection **"Pod 内多进程场景说明"** explaining:

- 当前版本支持多进程
- 原理（每个进程一个 project）
- 如何部署多容器 Pod 进行验证
- 如何确认多进程均生效：
  - `etmem project show -s etmemd_socket` 显示 N 个 project
  - 每个进程的 `/proc/<PID>/status` 中 VmSwap > 0

- [ ] **Step 4: Update 真实目标环境验证说明.md**

Add a note about multi-process verification in real environments:

- 如何区分"只生效一个进程"vs"多进程均生效"
- 检查方法：逐个进程查看 VmSwap
- 常见问题：进程名长度 > 15 字符被截断

- [ ] **Step 5: Commit documentation**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add docs/
git commit -m "docs: 添加多进程数据面验证说明及结果

说明 etmemd 的多进程限制（每配置文件一个 [task]，同名 project 拒绝重复添加），
Agent 采用每进程一个 project 的模型。包含多进程验证结果和操作指导。

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 7: Update dataplane verification script

**Files:**
- Modify: `scripts/verify-etmem-dataplane.sh`

- [ ] **Step 1: Add multi-process checks to the script**

Add a new section to `scripts/verify-etmem-dataplane.sh` that:

1. Counts the number of active etmemd projects
2. For each project, shows the config file contents
3. Identifies PIDs managed by each project
4. Reports VmRSS/VmSwap per process
5. Summarizes: "N processes managed across M projects"

The new section (add before the final summary):

```bash
echo ""
echo "=========================================="
echo " 第 10 步：多进程管理检查"
echo "=========================================="

# 统计 etmemd 管理的 project 数量
CONF_COUNT=$(ls /etc/etmem/*.conf 2>/dev/null | wc -l)
echo "[信息] 配置文件数量: $CONF_COUNT"

if [ "$CONF_COUNT" -gt 1 ]; then
    echo "[多进程] 检测到多个 project，逐个显示配置和 VmSwap："
    for CONF in /etc/etmem/*.conf; do
        PROJ_NAME=$(grep '^name=' "$CONF" | head -1 | cut -d= -f2)
        PROC_NAME=$(grep '^value=' "$CONF" | head -1 | cut -d= -f2)
        echo "  project=$PROJ_NAME  process=$PROC_NAME"
        if [ -n "$PROC_NAME" ]; then
            for P in $(pgrep -x "$PROC_NAME" 2>/dev/null); do
                SWAP=$(grep VmSwap /proc/$P/status 2>/dev/null | awk '{print $2}')
                RSS=$(grep VmRSS /proc/$P/status 2>/dev/null | awk '{print $2}')
                echo "    PID=$P  VmRSS=${RSS}kB  VmSwap=${SWAP}kB"
            done
        fi
    done
elif [ "$CONF_COUNT" -eq 1 ]; then
    echo "[单进程] 仅检测到一个 project"
else
    echo "[警告] 未发现任何 etmem 配置文件"
fi
```

- [ ] **Step 2: Run the script to verify it works**

Run: `bash scripts/verify-etmem-dataplane.sh`

Expected: Section 10 appears and reports process/project information.

- [ ] **Step 3: Commit**

```bash
cd /home/ygz/work/etmem-workspace/etmem-operator
git add scripts/verify-etmem-dataplane.sh
git commit -m "feat: 增强数据面诊断脚本支持多进程检查

新增第 10 步：多进程管理检查，逐个 project 显示配置和 VmSwap 状态。

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
