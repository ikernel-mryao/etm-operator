package transport

import "context"

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Transport defines the interface for communicating with etmemd.
type Transport interface {
	ObjAdd(ctx context.Context, configPath string) error
	ObjDel(ctx context.Context, configPath string) error
	ProjectStart(ctx context.Context, projectName string) error
	ProjectStop(ctx context.Context, projectName string) error
	ProjectShow(ctx context.Context, projectName string) (string, error)
}
