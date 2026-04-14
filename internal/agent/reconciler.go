// TaskManager 管理 etmem 任务的生命周期（启动/停止）。
// StopTask 容忍 transport 错误：etmemd 可能已停止或任务不存在，StopTask 需幂等。
// 即使 etmemd CLI 返回错误，仍删除本地配置文件和内存状态，避免泄漏。
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
	"github.com/openeuler/etmem-operator/internal/transport"
)

// TaskRequest contains the information needed to start an etmem task.
type TaskRequest struct {
	ProjectName   string
	ConfigContent string
}

// TaskManager manages the lifecycle of etmem tasks (start/stop via transport).
type TaskManager struct {
	transport transport.Transport
	configDir string
	mu        sync.Mutex
	running   map[string]string // projectName -> configPath
}

// NewTaskManager creates a new TaskManager.
func NewTaskManager(tr transport.Transport, configDir string) *TaskManager {
	return &TaskManager{transport: tr, configDir: configDir, running: make(map[string]string)}
}

// StartTask writes the config file, registers it with etmemd, and starts the project.
func (tm *TaskManager) StartTask(ctx context.Context, req TaskRequest) error {
	if err := os.MkdirAll(tm.configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	configPath := filepath.Join(tm.configDir, req.ProjectName+".conf")
	// etmemd 安全要求：配置文件权限必须为 600 或 400
	if err := os.WriteFile(configPath, []byte(req.ConfigContent), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := tm.transport.ObjAdd(ctx, configPath); err != nil {
		os.Remove(configPath)
		return fmt.Errorf("obj add: %w", err)
	}
	if err := tm.transport.ProjectStart(ctx, req.ProjectName); err != nil {
		_ = tm.transport.ObjDel(ctx, configPath)
		os.Remove(configPath)
		return fmt.Errorf("project start: %w", err)
	}
	tm.mu.Lock()
	tm.running[req.ProjectName] = configPath
	tm.mu.Unlock()
	return nil
}

// StopTask stops a running project, unregisters it, and removes the config file.
func (tm *TaskManager) StopTask(ctx context.Context, projectName string) error {
	tm.mu.Lock()
	configPath, exists := tm.running[projectName]
	if !exists {
		tm.mu.Unlock()
		return nil
	}
	delete(tm.running, projectName)
	tm.mu.Unlock()
	_ = tm.transport.ProjectStop(ctx, projectName)
	_ = tm.transport.ObjDel(ctx, configPath)
	os.Remove(configPath)
	return nil
}

// StopAll stops all running tasks.
func (tm *TaskManager) StopAll(ctx context.Context) {
	tm.mu.Lock()
	names := make([]string, 0, len(tm.running))
	for name := range tm.running {
		names = append(names, name)
	}
	tm.mu.Unlock()
	for _, name := range names {
		_ = tm.StopTask(ctx, name)
	}
}

// IsRunning returns true if the given project is currently running.
func (tm *TaskManager) IsRunning(projectName string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	_, ok := tm.running[projectName]
	return ok
}

// RunningTasks returns a snapshot of all currently running project names.
func (tm *TaskManager) RunningTasks() []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tasks := make([]string, 0, len(tm.running))
	for name := range tm.running {
		tasks = append(tasks, name)
	}
	return tasks
}

// BootstrapFromNodeState restores the running map from previously-reported NodeState tasks.
// Called once on Agent startup to avoid losing track of tasks that etmemd is still executing.
// Only tasks in "running" state with an existing config file are recovered.
func (tm *TaskManager) BootstrapFromNodeState(tasks []etmemv1.NodeTask) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	recovered := 0
	for _, task := range tasks {
		if task.State != "running" {
			continue
		}
		configPath := filepath.Join(tm.configDir, task.ProjectName+".conf")
		if _, err := os.Stat(configPath); err != nil {
			continue
		}
		tm.running[task.ProjectName] = configPath
		recovered++
	}
	return recovered
}
