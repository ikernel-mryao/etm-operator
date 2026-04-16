package engine

import (
	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

// ProcessTarget represents a process to be managed by etmem.
type ProcessTarget struct {
	Name string
}

// Engine defines the interface for etmem engine implementations.
type Engine interface {
	GenerateConfig(projectName string, process ProcessTarget, params SlideParams) string
	WriteConfigFile(dir, projectName string, process ProcessTarget, params SlideParams) (string, error)
}

// ApplyOverrides merges CRD-level overrides into base profile params.
func ApplyOverrides(base SlideParams, overrides *etmemv1.SlideOverrides) SlideParams {
	if overrides == nil {
		return base
	}
	if overrides.SwapThreshold != nil {
		base.SwapThreshold = *overrides.SwapThreshold
	}
	if overrides.SysMemThresholdPercent != nil {
		base.SysMemThreshold = *overrides.SysMemThresholdPercent
	}
	if overrides.Loop != nil {
		base.Loop = *overrides.Loop
	}
	if overrides.Interval != nil {
		base.Interval = *overrides.Interval
	}
	if overrides.Sleep != nil {
		base.Sleep = *overrides.Sleep
	}
	return base
}
