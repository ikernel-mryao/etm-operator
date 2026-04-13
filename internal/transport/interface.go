// Transport 抽象 etmemd 通信协议，支持可插拔实现。
// MVP 使用 exec 传输（直接调用 etmem CLI），未来可扩展为 socket 或 HTTP。
// 设计目的：解耦任务管理逻辑与底层通信机制。
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
