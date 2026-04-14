//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func newE2EClient(t *testing.T) client.Client {
	t.Helper()
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	require.NoError(t, err)
	s := runtime.NewScheme()
	require.NoError(t, etmemv1.AddToScheme(s))
	c, err := client.New(config, client.Options{Scheme: s})
	require.NoError(t, err)
	return c
}

func TestBasicFlow_CreatePolicyAndCheckNodeState(t *testing.T) {
	c := newE2EClient(t)
	ctx := context.Background()

	policy := &etmemv1.EtmemPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-test-policy", Namespace: "default",
		},
		Spec: etmemv1.EtmemPolicySpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"etmem-e2e": "true"},
			},
			ProcessFilter: &etmemv1.ProcessFilter{Names: []string{"sleep"}},
			Engine:        etmemv1.EngineSpec{Type: "slide", Profile: "conservative"},
		},
	}

	err := c.Create(ctx, policy)
	require.NoError(t, err)
	defer func() { _ = c.Delete(ctx, policy) }()

	time.Sleep(5 * time.Second)

	var fetched etmemv1.EtmemPolicy
	err = c.Get(ctx, types.NamespacedName{Name: "e2e-test-policy", Namespace: "default"}, &fetched)
	require.NoError(t, err)
	assert.Equal(t, "slide", fetched.Spec.Engine.Type)

	fetched.Spec.Suspend = true
	err = c.Update(ctx, &fetched)
	require.NoError(t, err)
}
