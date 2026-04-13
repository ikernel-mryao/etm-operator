package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

func AggregateForPolicy(ctx context.Context, c client.Client, namespace, name string) (*etmemv1.PolicySummary, error) {
	var nodeStates etmemv1.EtmemNodeStateList
	if err := c.List(ctx, &nodeStates); err != nil {
		return nil, err
	}
	summary := &etmemv1.PolicySummary{}
	nodeSet := make(map[string]bool)
	for _, ns := range nodeStates.Items {
		for _, task := range ns.Status.Tasks {
			if task.PolicyRef.Name == name && task.PolicyRef.Namespace == namespace {
				if task.State == "running" {
					summary.ManagedPods++
					nodeSet[ns.Status.NodeName] = true
				}
			}
		}
	}
	summary.ActiveNodes = len(nodeSet)
	return summary, nil
}
