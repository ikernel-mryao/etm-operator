package agent

import (
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
			ProcessFilter: etmemv1.ProcessFilter{Names: []string{"mysqld"}},
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
			ProcessFilter: etmemv1.ProcessFilter{Names: []string{"mysqld"}},
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
			ProcessFilter: etmemv1.ProcessFilter{Names: []string{"mysqld"}},
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
