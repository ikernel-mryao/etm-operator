// SlideEngine 负责生成 etmem slide 引擎的配置文件。
// 配置文件格式（INI 风格）：
//   [project] 定义全局扫描参数（loop、interval、内存阈值等）
//   [engine]  声明引擎类型为 slide
//   [task]    每个进程对应一个 task 块，定义换出策略（T、swap_threshold 等）
// 参数映射：Kubernetes API SlideParams → etmem CLI 配置字段
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SlideEngine struct{}

func (e *SlideEngine) GenerateConfig(projectName string, processes []ProcessTarget, params SlideParams) string {
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

	for i, proc := range processes {
		fmt.Fprintf(&b, "\n[task]\n")
		fmt.Fprintf(&b, "project=%s\n", projectName)
		fmt.Fprintf(&b, "engine=slide\n")
		fmt.Fprintf(&b, "name=%s_task_%d\n", projectName, i)
		fmt.Fprintf(&b, "type=name\n")
		fmt.Fprintf(&b, "value=%s\n", proc.Name)
		fmt.Fprintf(&b, "T=%d\n", params.T)
		fmt.Fprintf(&b, "max_threads=%d\n", params.MaxThreads)
		if params.SwapThreshold != "" {
			fmt.Fprintf(&b, "swap_threshold=%s\n", params.SwapThreshold)
		}
		fmt.Fprintf(&b, "swap_flag=%s\n", params.SwapFlag)
	}

	return b.String()
}

func (e *SlideEngine) WriteConfigFile(dir, projectName string, processes []ProcessTarget, params SlideParams) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	content := e.GenerateConfig(projectName, processes, params)
	path := filepath.Join(dir, projectName+".conf")
	// etmemd 安全要求：配置文件权限必须为 600 或 400
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("write config file: %w", err)
	}
	return path, nil
}
