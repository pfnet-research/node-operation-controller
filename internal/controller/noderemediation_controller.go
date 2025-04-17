/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
)

// NodeRemediationReconciler reconciles a NodeRemediation object
type NodeRemediationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	eventRecorder record.EventRecorder
}

var operationRemediationOwnerKey = "operationRemediationOwner"

// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations/finalizers,verbs=update

func (r *NodeRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	var err error

	var remediation nodeopsv1alpha1.NodeRemediation
	if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: remediation.Spec.NodeName}, &node); err != nil {
		return ctrl.Result{}, err
	}

	nodeStatus := remediation.CompareNodeCondition(node.Status.Conditions)
	if nodeStatus != remediation.Status.NodeStatus {
		remediation.Status.NodeStatus = nodeStatus
		if err := r.Status().Update(ctx, &remediation); err != nil {
			return ctrl.Result{}, err
		}
	}

	var childOps nodeopsv1alpha1.NodeOperationList
	if err := r.List(ctx, &childOps, client.MatchingFields{operationRemediationOwnerKey: remediation.Name}); err != nil {
		return ctrl.Result{}, err
	}

	var activeOp *nodeopsv1alpha1.NodeOperation
	for _, op := range childOps.Items {
		if op.Status.Phase == nodeopsv1alpha1.NodeOperationPhaseCompleted ||
			op.Status.Phase == nodeopsv1alpha1.NodeOperationPhaseFailed {
			continue
		}
		activeOp = &op
		break
	}

	var ref *corev1.ObjectReference
	if activeOp == nil {
		ref = &corev1.ObjectReference{}
	} else {
		ref, err = reference.GetReference(r.Scheme, activeOp)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	remediation.Status.ActiveNodeOperation = *ref
	if err := r.Status().Update(ctx, &remediation); err != nil {
		return ctrl.Result{}, err
	}

	// Check node condition
	switch remediation.Status.NodeStatus {
	case nodeopsv1alpha1.NodeStatusUnknown:
		r.eventRecorder.Eventf(&remediation, corev1.EventTypeNormal, "UnknownNodeStatus", "Because at least one Node condition is unknown status, remediation process is skipped")
		return ctrl.Result{}, nil
	case nodeopsv1alpha1.NodeStatusOK:
		// reset OperationsCount
		remediation.Status.OperationsCount = 0
		if err := r.Status().Update(ctx, &remediation); err != nil {
			return ctrl.Result{}, err
		}

		if ref := remediation.Status.ActiveNodeOperation; ref.Name != "" {
			// active operation exists
			var nodeOp nodeopsv1alpha1.NodeOperation
			if err := r.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &nodeOp); apierrors.IsNotFound(err) {
				// Do nothing
			} else if err != nil {
				return ctrl.Result{}, err
			} else {
				if err := r.Delete(ctx, &nodeOp); err != nil {
					return ctrl.Result{}, err
				}

				r.eventRecorder.Eventf(&remediation, corev1.EventTypeNormal, "DeleteNodeOperation", `Deleted NodeOperation %s because the Node is remediated`, nodeOp.Name)
			}

			remediation.Status.ActiveNodeOperation = corev1.ObjectReference{}
			if err := r.Status().Update(ctx, &remediation); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if remediation.Status.ActiveNodeOperation.Name != "" {
		// active operation exists
		return ctrl.Result{}, nil
	}

	// Avoid to create too many NodeOperations
	if 0 < remediation.Status.OperationsCount {
		// TODO: backoff feature.  We can calculate the next backoff-ed trial timestamp from the counter value and the latest child NodeOperation completion timestamp
		r.eventRecorder.Eventf(&remediation, corev1.EventTypeNormal, "NodeIsNotRemediated", `Though a NodeOperation has finished, the Node is not remediated. Skipping to create a NodeOperation.`)
		return ctrl.Result{}, nil
	}

	// Create nodeOperation
	var nodeOpTemplate nodeopsv1alpha1.NodeOperationTemplate
	if err := r.Get(ctx, types.NamespacedName{Name: remediation.Spec.NodeOperationTemplateName}, &nodeOpTemplate); err != nil {
		return ctrl.Result{}, err
	}

	opMeta := nodeOpTemplate.Spec.Template.Metadata.DeepCopy()
	if opMeta.Name == "" && opMeta.GenerateName == "" {
		opMeta.GenerateName = fmt.Sprintf("%s-", remediation.Name)
	}
	if opMeta.Labels == nil {
		opMeta.Labels = map[string]string{}
	}

	op := nodeopsv1alpha1.NodeOperation{
		ObjectMeta: *opMeta,
		Spec: nodeopsv1alpha1.NodeOperationSpec{
			NodeName:                  node.Name,
			NodeOperationSpecTemplate: nodeOpTemplate.Spec.Template.Spec,
		},
	}
	if err := ctrl.SetControllerReference(&remediation, &op, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, &op); err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Eventf(&remediation, corev1.EventTypeNormal, "CreatedNodeOperation", `Created a NodeOperation "%s"`, op.Name)

	// Update reference to NodeOperation
	ref, err = reference.GetReference(r.Scheme, &op)
	if err != nil {
		return ctrl.Result{}, err
	}
	remediation.Status.ActiveNodeOperation = *ref
	remediation.Status.OperationsCount++
	if err := r.Status().Update(ctx, &remediation); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeRemediationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("NodeRemediationControllerSetup")
	ctx := context.Background()

	r.eventRecorder = mgr.GetEventRecorderFor("node-operation-controller")

	if err := mgr.GetFieldIndexer().IndexField(ctx, &nodeopsv1alpha1.NodeOperation{}, operationRemediationOwnerKey, func(rawObj client.Object) []string {
		op := rawObj.(*nodeopsv1alpha1.NodeOperation)
		owner := metav1.GetControllerOf(op)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != nodeopsv1alpha1GVStr || owner.Kind != "NodeRemediation" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	nodeMapFn := func(ctx context.Context, obj client.Object) []reconcile.Request {
		nodeName := obj.GetName()

		remediations := &nodeopsv1alpha1.NodeRemediationList{}
		// TODO: use MatchingFields
		if err := r.List(ctx, remediations); err != nil {
			logger.Info("Failed to list NodeRemediations")
			return []reconcile.Request{}
		}

		var requests []reconcile.Request
		for _, remediation := range remediations.Items {
			if remediation.Spec.NodeName == nodeName {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: remediation.Name,
					},
				})
			}
		}

		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeopsv1alpha1.NodeRemediation{}).
		Owns(&nodeopsv1alpha1.NodeOperation{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(nodeMapFn)).
		Named("noderemediation").
		Complete(r)
}
