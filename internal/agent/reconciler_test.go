package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTransport struct {
	objAdds    []string
	objDels    []string
	projStarts []string
	projStops  []string
	projShows  []string
	showOutput string
	err        error
}

func (m *mockTransport) ObjAdd(ctx context.Context, configPath string) error {
	m.objAdds = append(m.objAdds, configPath)
	return m.err
}
func (m *mockTransport) ObjDel(ctx context.Context, configPath string) error {
	m.objDels = append(m.objDels, configPath)
	return m.err
}
func (m *mockTransport) ProjectStart(ctx context.Context, name string) error {
	m.projStarts = append(m.projStarts, name)
	return m.err
}
func (m *mockTransport) ProjectStop(ctx context.Context, name string) error {
	m.projStops = append(m.projStops, name)
	return m.err
}
func (m *mockTransport) ProjectShow(ctx context.Context, name string) (string, error) {
	m.projShows = append(m.projShows, name)
	return m.showOutput, m.err
}

func TestTaskManager_StartTask(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	err := tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0",
		ConfigContent: "[project]\nname=default-mysql-0\n",
	})
	require.NoError(t, err)
	assert.Len(t, tr.objAdds, 1)
	assert.Len(t, tr.projStarts, 1)
	assert.True(t, tm.IsRunning("default-mysql-0"))
}

func TestTaskManager_StopTask(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	_ = tm.StartTask(context.Background(), TaskRequest{
		ProjectName:   "default-mysql-0",
		ConfigContent: "[project]\nname=default-mysql-0\n",
	})
	err := tm.StopTask(context.Background(), "default-mysql-0")
	require.NoError(t, err)
	assert.Len(t, tr.projStops, 1)
	assert.Len(t, tr.objDels, 1)
	assert.False(t, tm.IsRunning("default-mysql-0"))
}

func TestTaskManager_StopNonexistent(t *testing.T) {
	tr := &mockTransport{}
	tm := NewTaskManager(tr, t.TempDir())
	err := tm.StopTask(context.Background(), "nonexistent")
	assert.NoError(t, err)
}
