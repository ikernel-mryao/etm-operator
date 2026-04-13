package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = etmemv1.AddToScheme(s)
	return s
}

func TestNodeStateWriter_CreateIfNotExists(t *testing.T) {
	s := newTestScheme()
	k8s := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&etmemv1.EtmemNodeState{}).Build()
	writer := NewNodeStateWriter(k8s, "worker-01")
	err := writer.WriteStatus(context.Background(), &etmemv1.EtmemNodeStateStatus{
		NodeName: "worker-01", EtmemdReady: true,
	})
	require.NoError(t, err)
	var ns etmemv1.EtmemNodeState
	err = k8s.Get(context.Background(), types.NamespacedName{Name: "worker-01"}, &ns)
	require.NoError(t, err)
	assert.Equal(t, "worker-01", ns.Status.NodeName)
	assert.True(t, ns.Status.EtmemdReady)
}

func TestNodeStateWriter_UpdateExisting(t *testing.T) {
	s := newTestScheme()
	existing := &etmemv1.EtmemNodeState{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-01"},
	}
	k8s := fake.NewClientBuilder().WithScheme(s).WithObjects(existing).WithStatusSubresource(existing).Build()
	writer := NewNodeStateWriter(k8s, "worker-01")
	err := writer.WriteStatus(context.Background(), &etmemv1.EtmemNodeStateStatus{
		NodeName: "worker-01", EtmemdReady: true,
		Tasks: []etmemv1.NodeTask{
			{ProjectName: "default-mysql-0", PodName: "mysql-0", State: "running"},
		},
		Metrics: &etmemv1.NodeMetrics{TotalManagedPods: 1},
	})
	require.NoError(t, err)
	var ns etmemv1.EtmemNodeState
	err = k8s.Get(context.Background(), types.NamespacedName{Name: "worker-01"}, &ns)
	require.NoError(t, err)
	assert.Len(t, ns.Status.Tasks, 1)
}
