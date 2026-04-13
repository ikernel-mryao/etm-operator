// MatchPodToPolicy 检查 Pod 是否应被该 EtmemPolicy 管理（在本节点上）。
// 匹配逻辑：namespace 相同 → Pod 调度到本节点 → nodeSelector 匹配 → labelSelector 匹配
package agent

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)
func MatchPodToPolicy(pod *corev1.Pod, policy *etmemv1.EtmemPolicy, nodeName string, nodeLabels map[string]string) bool {
	if policy.Spec.Suspend {
		return false
	}
	if pod.Namespace != policy.Namespace {
		return false
	}
	if pod.Spec.NodeName != nodeName {
		return false
	}
	for k, v := range policy.Spec.NodeSelector {
		if nodeLabels[k] != v {
			return false
		}
	}
	if policy.Spec.Selector != nil {
		selector, err := metav1.LabelSelectorAsSelector(policy.Spec.Selector)
		if err != nil {
			return false
		}
		if !selector.Matches(labels.Set(pod.Labels)) {
			return false
		}
	}
	return true
}

// PodTaskKey returns a unique key for a policy+pod combination.
func PodTaskKey(policyNamespace, policyName, podName string) string {
	return policyNamespace + "/" + policyName + "/" + podName
}

// ProjectName generates the etmem project name from namespace and pod name.
func ProjectName(namespace, podName string) string {
	name := namespace + "-" + podName
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}
