package controllers

import (
	"context"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type evictionStrategyProcessor struct {
	client        client.Client
	clientset     *kubernetes.Clientset
	eventRecorder record.EventRecorder
}

func newEvictionStrategyProcessor(client client.Client, clientset *kubernetes.Clientset, eventRecorder record.EventRecorder) *evictionStrategyProcessor {
	return &evictionStrategyProcessor{client: client, clientset: clientset, eventRecorder: eventRecorder}
}

// do performs eviction strategy specified in nodeOps against pods and returns all the pods are drained or not, or error
func (s *evictionStrategyProcessor) do(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	logger := log.FromContext(ctx)
	if len(pods) == 0 {
		return true, nil
	}
	switch nodeOp.Spec.EvictionStrategy {
	case nodeopsv1alpha1.NodeOperationEvictionStrategyDelete:
		return s.processDelete(ctx, pods, nodeOp)
	case nodeopsv1alpha1.NodeOperationEvictionStrategyForceDelete:
		return s.processForceDelete(ctx, pods, nodeOp)
	case nodeopsv1alpha1.NodeOperationEvictionStrategyNone:
		return s.processNone(ctx, pods, nodeOp)
	case nodeopsv1alpha1.NodeOperationEvictionStrategyEvict:
		return s.processEvict(ctx, pods, nodeOp)
	default:
		logger.Info("EvictionStrategy seems empty. Falling back to 'Evict' EvictionStrategy", "strategy", nodeOp.Spec.EvictionStrategy, "nodeoperation", nodeOp.Name)
		return s.processEvict(ctx, pods, nodeOp)
	}
}

func (s *evictionStrategyProcessor) processEvict(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("Performing EvictionStrategy", "strategy", nodeopsv1alpha1.NodeOperationEvictionStrategyEvict, "nodeoperation", nodeOp.Name)

	for _, pod := range pods {
		eviction := &policyv1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}

		logger.Info("Evicting a Pod", "namespace", pod.Namespace, "name", pod.Name, "nodeName", pod.Spec.NodeName)
		if err := s.clientset.CoreV1().Pods(pod.Namespace).Evict(ctx, eviction); err != nil {
			if errors.IsTooManyRequests(err) {
				logger.Info("Cannot do a Pod due to PDB", "namespace", pod.Namespace, "name", pod.Name, "nodeName", pod.Spec.NodeName)
				continue
			}
			return false, err
		}
		s.eventRecorder.Eventf(&pod, corev1.EventTypeNormal, "Evicted", `Node Operation "%s" evicted a Pod`, nodeOp.Name)
	}

	// returning false here to check no pods exists in next reconciliation round.
	return false, nil
}

func (s *evictionStrategyProcessor) processDelete(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("Performing EvictionStrategy", "strategy", nodeopsv1alpha1.NodeOperationEvictionStrategyDelete, "nodeoperation", nodeOp.Name)
	return s.deletePods(ctx, pods, nodeOp, false)
}

func (s *evictionStrategyProcessor) processForceDelete(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("Performing EvictionStrategy", "strategy", nodeopsv1alpha1.NodeOperationEvictionStrategyForceDelete, "nodeoperation", nodeOp.Name)
	return s.deletePods(ctx, pods, nodeOp, true)
}

func (s *evictionStrategyProcessor) deletePods(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation, force bool) (bool, error) {
	logger := log.FromContext(ctx)
	opts := []client.DeleteOption{}
	if force {
		opts = append(opts, client.GracePeriodSeconds(0))
	}

	for _, pod := range pods {
		logger.Info("Deleting a Pod", "namespace", pod.Namespace, "name", pod.Name, "nodeName", pod.Spec.NodeName)
		if err := s.client.Delete(context.Background(), &pod, opts...); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Pod Not found. Skip deletion", "namespace", pod.Namespace, "name", pod.Name, "nodeName", pod.Spec.NodeName)
				continue
			}
			logger.Error(err, "Couldn't Delete Pod", "namespace", pod.Namespace, "name", pod.Name, "nodeName", pod.Spec.NodeName)
			return false, err
		}
		if force {
			s.eventRecorder.Eventf(&pod, corev1.EventTypeNormal, "ForceDeleted", `Node Operation "%s" force deleted a Pod`, nodeOp.Name)
		} else {
			s.eventRecorder.Eventf(&pod, corev1.EventTypeNormal, "Deleted", `Node Operation "%s" deleted a Pod`, nodeOp.Name)
		}
	}

	// returning false here to check no pods exists in next reconciliation round.
	return false, nil
}

func (s *evictionStrategyProcessor) processNone(ctx context.Context, pods []corev1.Pod, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	logger := log.FromContext(ctx)
	logger.Info("Performing EvictionStrategy", "strategy", nodeopsv1alpha1.NodeOperationEvictionStrategyNone, "nodeoperation", nodeOp.Name)

	logger.Info("'None' EvictionStrategy performs nothing.")

	// returning false here to check no pods exists in next reconciliation round.
	return false, nil
}
