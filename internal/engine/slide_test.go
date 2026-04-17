package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

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

func TestSlideEngine_GenerateConfig_Moderate(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("moderate")
	process := ProcessTarget{Name: "mysqld", PID: 1234}

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
	assert.Contains(t, config, "value=1234")
	assert.Contains(t, config, "type=pid")
	assert.Contains(t, config, "T=1")
	assert.Contains(t, config, "swap_flag=no")
}

func TestSlideEngine_GenerateConfig_SingleProcess(t *testing.T) {
	e := &SlideEngine{}
	params, _ := GetProfile("conservative")
	process := ProcessTarget{Name: "java", PID: 5678}

	config := e.GenerateConfig("single-proj", process, params)

	taskCount := strings.Count(config, "[task]")
	assert.Equal(t, 1, taskCount, "etmemd only supports one [task] per config")
	assert.Contains(t, config, "value=5678")
	assert.Contains(t, config, "type=pid")
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
	process := ProcessTarget{Name: "mysqld", PID: 9999}

	path, err := e.WriteConfigFile(t.TempDir(), "test-proj", process, params)
	require.NoError(t, err)
	assert.Contains(t, path, "test-proj")
	assert.True(t, strings.HasSuffix(path, ".conf"))
}
