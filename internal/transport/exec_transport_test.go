package transport

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecutor struct {
	calls  []string
	err    error
	output []byte
}

func (m *mockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := name
	for _, a := range args {
		cmd += " " + a
	}
	m.calls = append(m.calls, cmd)
	return m.output, m.err
}

func TestExecTransport_ObjAdd(t *testing.T) {
	mock := &mockExecutor{}
	tr := NewExecTransport("etmemd_socket", mock)

	err := tr.ObjAdd(context.Background(), "/tmp/test.conf")
	require.NoError(t, err)
	require.Len(t, mock.calls, 1)
	assert.Equal(t, "/usr/bin/etmem obj add -f /tmp/test.conf -s etmemd_socket", mock.calls[0])
}

func TestExecTransport_ObjDel(t *testing.T) {
	mock := &mockExecutor{}
	tr := NewExecTransport("etmemd_socket", mock)

	err := tr.ObjDel(context.Background(), "/tmp/test.conf")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/etmem obj del -f /tmp/test.conf -s etmemd_socket", mock.calls[0])
}

func TestExecTransport_ProjectStart(t *testing.T) {
	mock := &mockExecutor{}
	tr := NewExecTransport("etmemd_socket", mock)

	err := tr.ProjectStart(context.Background(), "myproject")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/etmem project start -n myproject -s etmemd_socket", mock.calls[0])
}

func TestExecTransport_ProjectStop(t *testing.T) {
	mock := &mockExecutor{}
	tr := NewExecTransport("etmemd_socket", mock)

	err := tr.ProjectStop(context.Background(), "myproject")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/etmem project stop -n myproject -s etmemd_socket", mock.calls[0])
}

func TestExecTransport_ProjectShow(t *testing.T) {
	mock := &mockExecutor{output: []byte("project info output")}
	tr := NewExecTransport("etmemd_socket", mock)

	out, err := tr.ProjectShow(context.Background(), "myproject")
	require.NoError(t, err)
	assert.Equal(t, "project info output", out)
	assert.Equal(t, "/usr/bin/etmem project show -n myproject -s etmemd_socket", mock.calls[0])
}

func TestExecTransport_ErrorPropagation(t *testing.T) {
	mock := &mockExecutor{err: fmt.Errorf("connection refused")}
	tr := NewExecTransport("etmemd_socket", mock)

	err := tr.ObjAdd(context.Background(), "/tmp/test.conf")
	assert.ErrorContains(t, err, "connection refused")
}
