// Package v1alpha1 contains API Schema definitions for the etmem v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=etmem.openeuler.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "etmem.openeuler.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&EtmemPolicy{}, &EtmemPolicyList{})
	SchemeBuilder.Register(&EtmemNodeState{}, &EtmemNodeStateList{})
}
