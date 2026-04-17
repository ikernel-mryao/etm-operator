// MatchPodToPolicy 检查 Pod 是否应被该 EtmemPolicy 管理（在本节点上）。
// 匹配逻辑：namespace 相同 → Pod 调度到本节点 → nodeSelector 匹配 → labelSelector 匹配
package agent

import (
	"crypto/sha256"
	"encoding/hex"

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

// ProjectNameForProcess generates the etmem project name for a specific process in a pod.
// Each process needs its own project because etmemd rejects obj add for existing project names.
//
// Naming rule:
//   - Short names (≤64 chars): "{namespace}-{podName}-{processName}" — fully human-readable
//   - Long names (>64 chars):  "{55-char prefix}-{8-char SHA256 hex}" = 64 chars total
//     The hash is computed from the full untruncated name, so different inputs always
//     produce different outputs (with overwhelming probability).
//
// Properties:
//   - stable across reconcile loops (deterministic from inputs)
//   - unique per process within a pod (processName in name or hash)
//   - unique across pods (namespace + podName in name or hash)
//   - max 64 characters guaranteed
func ProjectNameForProcess(namespace, podName, processName string) string {
	full := namespace + "-" + podName + "-" + processName
	if len(full) <= 64 {
		return full
	}
	hash := sha256.Sum256([]byte(full))
	suffix := hex.EncodeToString(hash[:])[:8]
	return full[:55] + "-" + suffix
}
