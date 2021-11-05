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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
)

var (
	remediationOwnerKey  = "ownerNodeRemediationTemplate"
	nodeopsv1alpha1GVStr = nodeopsv1alpha1.GroupVersion.String()
)

// NodeRemediationTemplateReconciler reconciles a NodeRemediationTemplate object
type NodeRemediationTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	eventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediationtemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediationtemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=noderemediationtemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeRemediationTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *NodeRemediationTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var template nodeopsv1alpha1.NodeRemediationTemplate
	if err := r.Get(ctx, req.NamespacedName, &template); err != nil {
		sterr, ok := err.(*errors.StatusError)
		if ok && sterr.Status().Code == 404 {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var childRemediations nodeopsv1alpha1.NodeRemediationList
	if err := r.List(ctx, &childRemediations, client.MatchingFields{remediationOwnerKey: template.Name}); err != nil {
		return ctrl.Result{}, err
	}

	childRemediationByNodeName := map[string]*nodeopsv1alpha1.NodeRemediation{}
	for _, remediation := range childRemediations.Items {
		childRemediationByNodeName[remediation.Spec.NodeName] = &remediation
	}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return ctrl.Result{}, err
	}

	nodeSelector := labels.SelectorFromSet(template.Spec.NodeSelector)
	for _, node := range nodes.Items {
		if !nodeSelector.Matches(labels.Set(node.Labels)) {
			continue
		}

		if childRemediation, ok := childRemediationByNodeName[node.Name]; ok {
			// update if remediation exists

			// update labels
			if childRemediation.ObjectMeta.Labels == nil {
				childRemediation.ObjectMeta.Labels = map[string]string{}
			}
			for k, v := range template.Spec.Template.Metadata.Labels {
				childRemediation.ObjectMeta.Labels[k] = v
			}

			// update annotations
			if childRemediation.ObjectMeta.Annotations == nil {
				childRemediation.ObjectMeta.Annotations = map[string]string{}
			}
			for k, v := range template.Spec.Template.Metadata.Annotations {
				childRemediation.ObjectMeta.Annotations[k] = v
			}

			childRemediation.Spec.NodeRemediationSpecTemplate = template.Spec.Template.Spec
			if err := r.Update(ctx, childRemediation); err != nil {
				return ctrl.Result{}, nil
			}
		} else {
			// new remediation
			meta := template.Spec.Template.Metadata.DeepCopy()
			if meta.Name == "" && meta.GenerateName == "" {
				meta.GenerateName = fmt.Sprintf("%s-%s-", template.Name, node.Name)
			}
			remediation := nodeopsv1alpha1.NodeRemediation{
				ObjectMeta: *meta,
				Spec: nodeopsv1alpha1.NodeRemediationSpec{
					NodeRemediationSpecTemplate: template.Spec.Template.Spec,
					NodeName:                    node.Name,
				},
			}

			if err := ctrl.SetControllerReference(&template, &remediation, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.Create(ctx, &remediation); err != nil {
				return ctrl.Result{}, err
			}

			r.eventRecorder.Eventf(&template, corev1.EventTypeNormal, "CreatedRemediation", `Created a NodeRemediation "%s"`, remediation.Name)
		}

		delete(childRemediationByNodeName, node.Name)
	}

	for _, remediation := range childRemediationByNodeName {
		if err := r.Delete(ctx, remediation); err != nil {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeRemediationTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("NodeRemediationControllerSetup")
	ctx := context.Background()

	r.eventRecorder = mgr.GetEventRecorderFor("node-operation-controller")

	if err := mgr.GetFieldIndexer().IndexField(ctx, &nodeopsv1alpha1.NodeRemediation{}, remediationOwnerKey, func(rawObj client.Object) []string {
		remediation := rawObj.(*nodeopsv1alpha1.NodeRemediation)
		owner := metav1.GetControllerOf(remediation)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != nodeopsv1alpha1GVStr || owner.Kind != "NodeRemediationTemplate" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	nodeMapFn := func(a client.Object) []reconcile.Request {
		templates := &nodeopsv1alpha1.NodeRemediationTemplateList{}
		if err := r.List(context.TODO(), templates); err != nil {
			logger.Info("Failed to list NodeRemediationTemplates")
			return []reconcile.Request{}
		}

		nodeLabels := a.GetLabels()
		var requests []reconcile.Request

	nextTemplate:
		for _, template := range templates.Items {
			if !labels.SelectorFromSet(template.Spec.NodeSelector).Matches(labels.Set(nodeLabels)) {
				continue nextTemplate
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: template.Name,
				},
			})
		}

		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeopsv1alpha1.NodeRemediationTemplate{}).
		Watches(&source.Kind{Type: &corev1.Node{}}, handler.EnqueueRequestsFromMapFunc(nodeMapFn)).
		Complete(r)
}
