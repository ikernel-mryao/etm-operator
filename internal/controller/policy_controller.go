package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

type PolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=etmem.openeuler.io,resources=etmempolicies,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=etmem.openeuler.io,resources=etmempolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=etmem.openeuler.io,resources=etmemnodestates,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy etmemv1.EtmemPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("EtmemPolicy not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling EtmemPolicy", "name", policy.Name, "namespace", policy.Namespace)

	// Phase 1 skeleton: validates policy exists and logs reconcile.
	// Full status aggregation is wired in Task 3.3.
	// The Agent (not Operator) handles actual etmem task lifecycle.
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etmemv1.EtmemPolicy{}).
		Complete(r)
}
