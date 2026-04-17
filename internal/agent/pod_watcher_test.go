package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func TestMatchPodToPolicy_LabelSelector(t *testing.T) {
	policy := &etmemv1.EtmemPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: etmemv1.EtmemPolicySpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "mysql"},
			},
			NodeSelector:  map[string]string{"node-role": "worker"},
			ProcessFilter: &etmemv1.ProcessFilter{Names: []string{"mysqld"}},
			Engine:        etmemv1.EngineSpec{Type: "slide", Profile: "moderate"},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mysql-0", Namespace: "default",
			Labels: map[string]string{"app": "mysql"},
		},
		Spec: corev1.PodSpec{NodeName: "worker-01"},
	}
	nodeLabels := map[string]string{"node-role": "worker"}
	assert.True(t, MatchPodToPolicy(pod, policy, "worker-01", nodeLabels))
}

func TestMatchPodToPolicy_WrongNamespace(t *testing.T) {
	policy := &etmemv1.EtmemPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "prod"},
		Spec: etmemv1.EtmemPolicySpec{
			Selector:      &metav1.LabelSelector{MatchLabels: map[string]string{"app": "mysql"}},
			ProcessFilter: &etmemv1.ProcessFilter{Names: []string{"mysqld"}},
			Engine:        etmemv1.EngineSpec{Type: "slide"},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-0", Namespace: "default", Labels: map[string]string{"app": "mysql"}},
		Spec: corev1.PodSpec{NodeName: "worker-01"},
	}
	assert.False(t, MatchPodToPolicy(pod, policy, "worker-01", nil))
}

func TestMatchPodToPolicy_Suspended(t *testing.T) {
	policy := &etmemv1.EtmemPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: etmemv1.EtmemPolicySpec{
			Selector:      &metav1.LabelSelector{MatchLabels: map[string]string{"app": "mysql"}},
			ProcessFilter: &etmemv1.ProcessFilter{Names: []string{"mysqld"}},
			Engine:        etmemv1.EngineSpec{Type: "slide"},
			Suspend:       true,
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mysql-0", Namespace: "default", Labels: map[string]string{"app": "mysql"}},
		Spec: corev1.PodSpec{NodeName: "worker-01"},
	}
	assert.False(t, MatchPodToPolicy(pod, policy, "worker-01", nil))
}

func TestProjectNameForProcess(t *testing.T) {
	name := ProjectNameForProcess("default", "mysql-0", "mysqld", 1234)
	assert.Equal(t, "default-mysql-0-mysqld-p1234", name)
}

func TestProjectNameForProcess_Truncation(t *testing.T) {
	long := strings.Repeat("a", 60)
	name := ProjectNameForProcess(long, "pod-name", "proc", 1234)
	assert.LessOrEqual(t, len(name), 64)
	// Truncated names must contain a hash suffix (last 8 hex chars after '-')
	assert.Equal(t, 64, len(name), "truncated names should be exactly 64 chars")
}

func TestProjectNameForProcess_Stable(t *testing.T) {
	// Same inputs must always produce the same output (reconcile stability)
	a := ProjectNameForProcess("ns", "pod", "proc", 1234)
	b := ProjectNameForProcess("ns", "pod", "proc", 1234)
	assert.Equal(t, a, b)

	// Stability must also hold for truncated names
	long := strings.Repeat("x", 60)
	c := ProjectNameForProcess(long, "pod", "proc", 1234)
	d := ProjectNameForProcess(long, "pod", "proc", 1234)
	assert.Equal(t, c, d)
}

func TestProjectNameForProcess_UniqueAcrossProcesses(t *testing.T) {
	// Different process names must produce different project names
	a := ProjectNameForProcess("default", "mysql-0", "mysqld", 1234)
	b := ProjectNameForProcess("default", "mysql-0", "java", 1234)
	assert.NotEqual(t, a, b)
}

func TestProjectNameForProcess_UniqueAcrossPods(t *testing.T) {
	// Different pods must produce different project names for same process
	a := ProjectNameForProcess("default", "mysql-0", "mysqld", 1234)
	b := ProjectNameForProcess("default", "mysql-1", "mysqld", 1234)
	assert.NotEqual(t, a, b)
}

func TestProjectNameForProcess_TruncationCollisionResistance(t *testing.T) {
	// With long namespace+podName, different processes must still produce
	// different project names after truncation. This was a bug: blind
	// truncation at 64 chars caused proc1 and proc2 to collide.
	longNs := strings.Repeat("a", 30)
	longPod := strings.Repeat("b", 28)
	// Full name = 30 + 1 + 28 + 1 + 5 + 1 + 5 = 71 chars → triggers truncation
	a := ProjectNameForProcess(longNs, longPod, "proc1", 1234)
	b := ProjectNameForProcess(longNs, longPod, "proc2", 1234)
	assert.NotEqual(t, a, b, "different processes must not collide after truncation")
	assert.LessOrEqual(t, len(a), 64)
	assert.LessOrEqual(t, len(b), 64)
}

func TestProjectNameForProcess_ShortNameFullyReadable(t *testing.T) {
	// Short names must be the exact concatenation with no hash suffix
	name := ProjectNameForProcess("ns", "pod", "proc", 1234)
	assert.Equal(t, "ns-pod-proc-p1234", name)
}

func TestProjectNameForProcess_ExactBoundary(t *testing.T) {
	// Name that is exactly 64 chars should NOT get hashed
	// 64 - 3 (separators) - 6 ("-p1234") = 55 chars for parts
	ns := strings.Repeat("n", 18)
	pod := strings.Repeat("p", 18)
	proc := strings.Repeat("r", 19) // 18 + 1 + 18 + 1 + 19 + 6 = 63
	name := ProjectNameForProcess(ns, pod, proc, 1234)
	expected := ns + "-" + pod + "-" + proc + "-p1234"
	assert.Equal(t, expected, name, "exactly 63 chars should use full name")
	assert.Equal(t, 63, len(name))
}

func TestProjectNameForProcess_UniqueAcrossPIDs(t *testing.T) {
	// Same namespace, pod, and process but different PIDs must produce different project names
	a := ProjectNameForProcess("default", "mysql-0", "mysqld", 1234)
	b := ProjectNameForProcess("default", "mysql-0", "mysqld", 5678)
	assert.NotEqual(t, a, b, "different PIDs must produce different project names")
	
	// Also test with truncation case
	long := strings.Repeat("a", 50)
	c := ProjectNameForProcess(long, "pod", "proc", 1234)
	d := ProjectNameForProcess(long, "pod", "proc", 5678)
	assert.NotEqual(t, c, d, "different PIDs must produce different truncated names")
}

func TestProjectNameForProcess_PIDInName(t *testing.T) {
	// Verify the project name contains -p{pid} for short names
	name := ProjectNameForProcess("ns", "pod", "proc", 9876)
	assert.Contains(t, name, "-p9876", "project name should contain PID suffix")
	assert.Equal(t, "ns-pod-proc-p9876", name)
}
