package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = etmemv1.AddToScheme(s)
	return s
}

func TestPolicyReconciler_NotFound(t *testing.T) {
	s := newScheme()
	client := fake.NewClientBuilder().WithScheme(s).Build()
	r := &PolicyReconciler{Client: client, Scheme: s}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestPolicyReconciler_FetchesPolicy(t *testing.T) {
	s := newScheme()
	policy := &etmemv1.EtmemPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Namespace: "default"},
		Spec: etmemv1.EtmemPolicySpec{
			ProcessFilter: etmemv1.ProcessFilter{Names: []string{"mysqld"}},
			Engine:        etmemv1.EngineSpec{Type: "slide", Profile: "moderate"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(s).WithObjects(policy).WithStatusSubresource(policy).Build()
	r := &PolicyReconciler{Client: client, Scheme: s}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "default"},
	})
	require.NoError(t, err)
	// When policy is found, reconciler returns RequeueAfter 30s
	assert.Equal(t, ctrl.Result{RequeueAfter: 30 * time.Second}, result)
}
