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
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write config file: %w", err)
	}
	return path, nil
}
