/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var operationRemediationOwnerKey = "operationRemediationOwner"

// NodeRemediationReconciler reconciles a NodeRemediation object
type NodeRemediationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	eventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeRemediation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *NodeRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	var err error

	var remediation nodeopsv1alpha1.NodeRemediation
	if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
		sterr, ok := err.(*errors.StatusError)
		if ok && sterr.Status().Code == 404 {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
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

	if remediation.Status.ActiveNodeOperation.Name != "" {
		// active operation exists
		return ctrl.Result{}, nil
	}

	// Check node condition
	switch remediation.Status.NodeStatus {
	case nodeopsv1alpha1.NodeStatusUnknown:
		r.eventRecorder.Eventf(&remediation, corev1.EventTypeNormal, "UnknownNodeStatus", "Because at least one Node condition is unknown status, remediation process is skipped")
		return ctrl.Result{}, nil
	case nodeopsv1alpha1.NodeStatusOK:
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

	nodeMapFn := func(a client.Object) []reconcile.Request {
		nodeName := a.GetName()

		remediations := &nodeopsv1alpha1.NodeRemediationList{}
		// TODO: use MatchingFields
		if err := r.List(context.TODO(), remediations); err != nil {
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
		Watches(&source.Kind{Type: &corev1.Node{}}, handler.EnqueueRequestsFromMapFunc(nodeMapFn)).
		Complete(r)
}
