package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func TestAggregateNodeStates(t *testing.T) {
	s := newScheme()
	ns1 := &etmemv1.EtmemNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-01"},
		Status: etmemv1.EtmemNodeStateStatus{
			NodeName: "worker-01",
			Tasks: []etmemv1.NodeTask{
				{PolicyRef: etmemv1.PolicyReference{Name: "my-policy", Namespace: "default"}, PodName: "mysql-0", State: "running"},
			},
		},
	}
	ns2 := &etmemv1.EtmemNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-02"},
		Status: etmemv1.EtmemNodeStateStatus{
			NodeName: "worker-02",
			Tasks: []etmemv1.NodeTask{
				{PolicyRef: etmemv1.PolicyReference{Name: "my-policy", Namespace: "default"}, PodName: "mysql-1", State: "running"},
				{PolicyRef: etmemv1.PolicyReference{Name: "other-policy", Namespace: "prod"}, PodName: "app-0", State: "running"},
			},
		},
	}
	k8s := fake.NewClientBuilder().WithScheme(s).WithObjects(ns1, ns2).Build()
	summary, err := AggregateForPolicy(context.Background(), k8s, "default", "my-policy")
	require.NoError(t, err)
	assert.Equal(t, 2, summary.ActiveNodes)
	assert.Equal(t, 2, summary.ManagedPods)
}

func TestAggregateNodeStates_NoPods(t *testing.T) {
	s := newScheme()
	k8s := fake.NewClientBuilder().WithScheme(s).Build()
	summary, err := AggregateForPolicy(context.Background(), k8s, "default", "my-policy")
	require.NoError(t, err)
	assert.Equal(t, 0, summary.ActiveNodes)
	assert.Equal(t, 0, summary.ManagedPods)
}
