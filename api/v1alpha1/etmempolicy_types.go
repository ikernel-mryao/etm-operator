package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.engine.profile`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Active Nodes",type=integer,JSONPath=`.status.summary.activeNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EtmemPolicy defines a memory tiering policy for target workloads.
type EtmemPolicy struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EtmemPolicySpec   `json:"spec,omitempty"`
    Status EtmemPolicyStatus `json:"status,omitempty"`
}

type EtmemPolicySpec struct {
    // Selector selects Pods by labels. Mutually exclusive with WorkloadRefs.
    // +optional
    Selector *metav1.LabelSelector `json:"selector,omitempty"`

    // WorkloadRefs references specific workloads. Mutually exclusive with Selector.
    // +optional
    WorkloadRefs []WorkloadRef `json:"workloadRefs,omitempty"`

    // NodeSelector limits which nodes this policy applies to.
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // ProcessFilter specifies which processes to manage within matched containers.
    ProcessFilter ProcessFilter `json:"processFilter"`

    // Engine configures the etmem engine parameters.
    Engine EngineSpec `json:"engine"`

    // Suspend stops all tasks for this policy when true.
    // +optional
    Suspend bool `json:"suspend,omitempty"`

    // CircuitBreaker configures automatic safety thresholds.
    // +optional
    CircuitBreaker *CircuitBreakerSpec `json:"circuitBreaker,omitempty"`
}

type WorkloadRef struct {
    // APIGroup is the API group of the referenced workload (e.g. "apps").
    APIGroup string `json:"apiGroup"`
    // Kind is the kind of the referenced workload (e.g. "Deployment", "DaemonSet").
    Kind string `json:"kind"`
    // Name is the name of the referenced workload.
    Name string `json:"name"`
}

type ProcessFilter struct {
    // Names is the list of process names to manage (matched against /proc/<pid>/comm).
    // Each name must be <= 15 characters.
    // +kubebuilder:validation:MinItems=1
    Names []string `json:"names"`
}

type EngineSpec struct {
    // Type is the engine type. MVP only supports "slide".
    // +kubebuilder:validation:Enum=slide
    // +kubebuilder:default=slide
    Type string `json:"type"`

    // Profile is the predefined parameter set: conservative, moderate, or aggressive.
    // +kubebuilder:validation:Enum=conservative;moderate;aggressive
    // +kubebuilder:default=moderate
    // +optional
    Profile string `json:"profile,omitempty"`

    // Overrides allows overriding specific profile parameters.
    // +optional
    Overrides *SlideOverrides `json:"overrides,omitempty"`
}

type SlideOverrides struct {
    // SwapThreshold is the swap threshold (e.g. "10g").
    // +optional
    SwapThreshold *string `json:"swapThreshold,omitempty"`
    // SysMemThresholdPercent overrides system memory threshold percentage.
    // +optional
    SysMemThresholdPercent *int `json:"sysMemThresholdPercent,omitempty"`
    // Loop overrides the scan loop count.
    // +optional
    Loop *int `json:"loop,omitempty"`
    // Interval overrides the scan interval in seconds.
    // +optional
    Interval *int `json:"interval,omitempty"`
    // Sleep overrides the sleep time between scans in seconds.
    // +optional
    Sleep *int `json:"sleep,omitempty"`
}

type CircuitBreakerSpec struct {
    // PodRestartThreshold triggers circuit breaker when pod restart count exceeds this.
    // +kubebuilder:default=3
    // +optional
    PodRestartThreshold *int `json:"podRestartThreshold,omitempty"`

    // NodeMemoryPSIThresholdPercent triggers node-level circuit breaker when PSI exceeds this.
    // +kubebuilder:default=80
    // +optional
    NodeMemoryPSIThresholdPercent *int `json:"nodeMemoryPSIThresholdPercent,omitempty"`
}

type EtmemPolicyStatus struct {
    // Conditions represent the latest available observations.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Summary provides aggregated statistics across all nodes.
    // +optional
    Summary *PolicySummary `json:"summary,omitempty"`
}

type PolicySummary struct {
    ActiveNodes     int    `json:"activeNodes"`
    ManagedPods     int    `json:"managedPods"`
    TotalSwappedBytes string `json:"totalSwappedBytes,omitempty"`
}

// +kubebuilder:object:root=true

// EtmemPolicyList contains a list of EtmemPolicy.
type EtmemPolicyList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []EtmemPolicy `json:"items"`
}
