// NodeStateWriter 负责写入 EtmemNodeState 状态资源。
// 采用 create-then-update 模式：首次写入先创建对象，再更新 status 子资源。
// Kubernetes 要求 status 子资源必须通过 client.Status().Update() 单独更新。
package agent

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etmemv1 "github.com/openeuler/etmem-operator/api/v1alpha1"
)

type NodeStateWriter struct {
	client   client.Client
	nodeName string
}

func NewNodeStateWriter(c client.Client, nodeName string) *NodeStateWriter {
	return &NodeStateWriter{client: c, nodeName: nodeName}
}

func (w *NodeStateWriter) WriteStatus(ctx context.Context, status *etmemv1.EtmemNodeStateStatus) error {
	var ns etmemv1.EtmemNodeState
	err := w.client.Get(ctx, types.NamespacedName{Name: w.nodeName}, &ns)
	if errors.IsNotFound(err) {
		ns = etmemv1.EtmemNodeState{
			ObjectMeta: metav1.ObjectMeta{Name: w.nodeName},
		}
		if err := w.client.Create(ctx, &ns); err != nil {
			return fmt.Errorf("create EtmemNodeState: %w", err)
		}
		ns.Status = *status
		if err := w.client.Status().Update(ctx, &ns); err != nil {
			return fmt.Errorf("update EtmemNodeState status after create: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get EtmemNodeState: %w", err)
	}
	ns.Status = *status
	if err := w.client.Status().Update(ctx, &ns); err != nil {
		return fmt.Errorf("update EtmemNodeState status: %w", err)
	}
	return nil
}
