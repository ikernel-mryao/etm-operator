package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Tasks",type=integer,JSONPath=`.status.metrics.totalManagedPods`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EtmemNodeState reports per-node observed state for etmem tasks.
type EtmemNodeState struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EtmemNodeStateSpec   `json:"spec,omitempty"`
    Status EtmemNodeStateStatus `json:"status,omitempty"`
}

// EtmemNodeStateSpec is intentionally empty.
type EtmemNodeStateSpec struct{}

type EtmemNodeStateStatus struct {
    NodeName string `json:"nodeName,omitempty"`
    EtmemdReady bool `json:"etmemdReady"`
    SocketReachable bool `json:"socketReachable"`
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    // +optional
    Tasks []NodeTask `json:"tasks,omitempty"`
    // +optional
    Metrics *NodeMetrics `json:"metrics,omitempty"`
}

type NodeTask struct {
    PolicyRef PolicyReference `json:"policyRef"`
    PodName string `json:"podName"`
    PodUID string `json:"podUID"`
    ContainerName string `json:"containerName,omitempty"`
    // +optional
    Processes []ManagedProcess `json:"processes,omitempty"`
    ProjectName string `json:"projectName"`
    // +kubebuilder:validation:Enum=running;stopped;error;circuit-broken
    State string `json:"state"`
    // +optional
    LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
    // +optional
    Error string `json:"error,omitempty"`
}

type PolicyReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type ManagedProcess struct {
    PID          int    `json:"pid"`
    Name         string `json:"name"`
    RSSBytes     string `json:"rssBytes,omitempty"`
    SwappedBytes string `json:"swappedBytes,omitempty"`
}

type NodeMetrics struct {
    TotalManagedPods       int    `json:"totalManagedPods"`
    TotalSwappedBytes      string `json:"totalSwappedBytes,omitempty"`
    NodeMemoryPSIPercent   int    `json:"nodeMemoryPSIPercent,omitempty"`
}

// +kubebuilder:object:root=true

type EtmemNodeStateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []EtmemNodeState `json:"items"`
}
