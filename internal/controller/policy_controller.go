// PolicyReconciler 负责监听 EtmemPolicy 并聚合状态。
// 职责：聚合各节点 EtmemNodeState 状态到 Policy.Status，不管理任务生命周期。
// 任务生命周期由 Agent 自治管理，Operator 仅提供全局视图。
package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Validate: WorkloadRefs not supported in MVP
	if len(policy.Spec.WorkloadRefs) > 0 {
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "WorkloadRefsNotSupported",
			Message:            "WorkloadRefs is not supported in MVP. Use selector (labelSelector) instead.",
			LastTransitionTime: metav1.Now(),
		}
		setCondition(&policy.Status.Conditions, condition)
		if err := r.Status().Update(ctx, &policy); err != nil {
			logger.Error(err, "failed to update policy status")
			return ctrl.Result{}, err
		}
		logger.Info("Policy rejected: WorkloadRefs not supported in MVP", "policy", policy.Name)
		return ctrl.Result{}, nil
	}

	summary, err := AggregateForPolicy(ctx, r.Client, policy.Namespace, policy.Name)
	if err != nil {
		logger.Error(err, "failed to aggregate NodeState")
		return ctrl.Result{}, err
	}
	policy.Status.Summary = summary
	if err := r.Status().Update(ctx, &policy); err != nil {
		logger.Error(err, "failed to update policy status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etmemv1.EtmemPolicy{}).
		Complete(r)
}

func setCondition(conditions *[]metav1.Condition, cond metav1.Condition) {
	for i, existing := range *conditions {
		if existing.Type == cond.Type {
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}
