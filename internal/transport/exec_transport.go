package transport

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/openeuler/etmem-operator/internal/config"
)

// RealExecutor executes commands via os/exec.
type RealExecutor struct{}

func (e *RealExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// ExecTransport implements Transport by shelling out to the etmem CLI.
type ExecTransport struct {
	socketName string
	executor   CommandExecutor
}

func NewExecTransport(socketName string, executor CommandExecutor) *ExecTransport {
	return &ExecTransport{socketName: socketName, executor: executor}
}

func (t *ExecTransport) ObjAdd(ctx context.Context, configPath string) error {
	_, err := t.executor.Execute(ctx, config.EtmemBinaryPath, "obj", "add", "-f", configPath, "-s", t.socketName)
	if err != nil {
		return fmt.Errorf("etmem obj add failed: %w", err)
	}
	return nil
}

func (t *ExecTransport) ObjDel(ctx context.Context, configPath string) error {
	_, err := t.executor.Execute(ctx, config.EtmemBinaryPath, "obj", "del", "-f", configPath, "-s", t.socketName)
	if err != nil {
		return fmt.Errorf("etmem obj del failed: %w", err)
	}
	return nil
}

func (t *ExecTransport) ProjectStart(ctx context.Context, projectName string) error {
	_, err := t.executor.Execute(ctx, config.EtmemBinaryPath, "project", "start", "-n", projectName, "-s", t.socketName)
	if err != nil {
		return fmt.Errorf("etmem project start failed: %w", err)
	}
	return nil
}

func (t *ExecTransport) ProjectStop(ctx context.Context, projectName string) error {
	_, err := t.executor.Execute(ctx, config.EtmemBinaryPath, "project", "stop", "-n", projectName, "-s", t.socketName)
	if err != nil {
		return fmt.Errorf("etmem project stop failed: %w", err)
	}
	return nil
}

func (t *ExecTransport) ProjectShow(ctx context.Context, projectName string) (string, error) {
	out, err := t.executor.Execute(ctx, config.EtmemBinaryPath, "project", "show", "-n", projectName, "-s", t.socketName)
	if err != nil {
		return "", fmt.Errorf("etmem project show failed: %w", err)
	}
	return string(out), nil
}
