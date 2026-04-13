// Profile 预设提供三种开箱即用的内存分级策略：
// conservative：保守策略，适用生产核心业务（低扫描频率、高内存阈值）
// moderate：中庸策略，适用通用场景（默认，平衡性能和内存回收）
// aggressive：激进策略，适用批处理/离线任务（高扫描频率、低内存阈值）
package engine

import (
	"fmt"
	"sort"
)

// SlideParams holds the complete parameter set for a slide engine task.
type SlideParams struct {
	Loop               int
	Interval           int
	Sleep              int
	SysMemThreshold    int
	SwapCacheHighWmark int
	SwapCacheLowWmark  int
	T                  int
	MaxThreads         int
	SwapFlag           string
	SwapThreshold      string // e.g. "10g"
}

var profiles = map[string]SlideParams{
	"conservative": {
		Loop: 3, Interval: 5, Sleep: 3,
		SysMemThreshold: 70, SwapCacheHighWmark: 15, SwapCacheLowWmark: 10,
		T: 3, MaxThreads: 1, SwapFlag: "yes", SwapThreshold: "10g",
	},
	"moderate": {
		Loop: 1, Interval: 1, Sleep: 1,
		SysMemThreshold: 50, SwapCacheHighWmark: 10, SwapCacheLowWmark: 6,
		T: 1, MaxThreads: 1, SwapFlag: "yes", SwapThreshold: "10g",
	},
	"aggressive": {
		Loop: 1, Interval: 1, Sleep: 0,
		SysMemThreshold: 30, SwapCacheHighWmark: 5, SwapCacheLowWmark: 3,
		T: 1, MaxThreads: 2, SwapFlag: "yes", SwapThreshold: "10g",
	},
}

// GetProfile returns the SlideParams for a named profile.
func GetProfile(name string) (SlideParams, error) {
	p, ok := profiles[name]
	if !ok {
		return SlideParams{}, fmt.Errorf("unknown profile %q, valid: %v", name, ValidProfileNames())
	}
	return p, nil
}

// ValidProfileNames returns sorted list of valid profile names.
func ValidProfileNames() []string {
	names := make([]string, 0, len(profiles))
	for k := range profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
