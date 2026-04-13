package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCircuitBreaker_PodLevel_NoTrip(t *testing.T) {
	cb := NewCircuitBreaker(3, 80, "")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-0"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 1}},
		},
	}
	assert.False(t, cb.IsPodTripped(pod))
}

func TestCircuitBreaker_PodLevel_Tripped(t *testing.T) {
	cb := NewCircuitBreaker(3, 80, "")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-0"},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 5}},
		},
	}
	assert.True(t, cb.IsPodTripped(pod))
}

func TestCircuitBreaker_NodeLevel_PSI_Tripped(t *testing.T) {
	tmpDir := t.TempDir()
	psiPath := filepath.Join(tmpDir, "memory")
	psiContent := "some avg10=85.00 avg60=70.00 avg300=50.00 total=12345\nfull avg10=30.00 avg60=20.00 avg300=10.00 total=6789\n"
	os.WriteFile(psiPath, []byte(psiContent), 0644)

	cb := NewCircuitBreaker(3, 80, tmpDir)
	tripped, psiValue := cb.IsNodeTripped()
	assert.True(t, tripped)
	assert.Equal(t, 85, psiValue)
}

func TestCircuitBreaker_NodeLevel_NotTripped(t *testing.T) {
	tmpDir := t.TempDir()
	psiPath := filepath.Join(tmpDir, "memory")
	psiContent := "some avg10=30.00 avg60=25.00 avg300=20.00 total=12345\nfull avg10=10.00 avg60=5.00 avg300=2.00 total=6789\n"
	os.WriteFile(psiPath, []byte(psiContent), 0644)

	cb := NewCircuitBreaker(3, 80, tmpDir)
	tripped, _ := cb.IsNodeTripped()
	assert.False(t, tripped)
}
