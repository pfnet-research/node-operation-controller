/*

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
	"github.com/go-logr/logr"
	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"
)

var controllerTaint = corev1.Taint{
	Key:    "nodeops.k8s.preferred.jp/operating",
	Effect: "NoSchedule",
	Value:  "",
}

const jobOwnerKey = ".metadata.controller"
const eventSourceName = "node-operation-controller"

// NodeOperationReconciler reconciles a NodeOperation object
type NodeOperationReconciler struct {
	client.Client
	Log              logr.Logger
	Scheme           *runtime.Scheme
	DrainInterval    time.Duration
	NDBRetryInterval time.Duration

	clientset                 *kubernetes.Clientset
	mutex                     sync.Mutex
	eventRecorder             record.EventRecorder
	evictionStrategyProcessor *evictionStrategyProcessor
}

// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=nodeoperations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=nodeoperations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=nodedisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=nodeops.k8s.preferred.jp,resources=nodedisruptionbudgets/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *NodeOperationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("nodeoperation", req.NamespacedName)

	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	if err := r.Get(ctx, req.NamespacedName, nodeOp); err != nil {
		sterr, ok := err.(*errors.StatusError)
		if ok && sterr.Status().Code == 404 {
			if err := r.removeTaints(ctx); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var result ctrl.Result
	var err error

	prevPhase := nodeOp.Status.Phase
	switch nodeOp.Status.Phase {
	case "":
		nodeOp.Status.Reason = "NodeOperation is created"
		nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhasePending
		if err := r.Update(ctx, nodeOp); err != nil {
			return ctrl.Result{}, err
		}
	case nodeopsv1alpha1.NodeOperationPhasePending:
		result, err = r.reconcilePending(ctx, nodeOp)
	case nodeopsv1alpha1.NodeOperationPhaseDraining:
		result, err = r.reconcileDraining(ctx, nodeOp)
	case nodeopsv1alpha1.NodeOperationPhaseDrained:
		result, err = r.reconcileDrained(ctx, nodeOp)
	case nodeopsv1alpha1.NodeOperationPhaseJobCreating:
		result, err = r.reconcileJobCreating(ctx, nodeOp)
	case nodeopsv1alpha1.NodeOperationPhaseRunning:
		result, err = r.reconcileRunning(ctx, nodeOp)
	default:
		return ctrl.Result{}, nil
	}
	r.Log.Info("phase changed", "name", nodeOp.Name, "from", prevPhase, "to", nodeOp.Status.Phase)

	return result, err
}

func (r *NodeOperationReconciler) removeTaints(ctx context.Context) error {
	nodeOpList := nodeopsv1alpha1.NodeOperationList{}
	if err := r.List(ctx, &nodeOpList); err != nil {
		return err
	}

	activeNodeNames := map[string]struct{}{}
	for _, op := range nodeOpList.Items {
		phase := op.Status.Phase
		if phase == nodeopsv1alpha1.NodeOperationPhaseDrained ||
			phase == nodeopsv1alpha1.NodeOperationPhaseDraining ||
			phase == nodeopsv1alpha1.NodeOperationPhaseRunning {
			activeNodeNames[op.Spec.NodeName] = struct{}{}
		}
	}

	nodeList := corev1.NodeList{}
	if err := r.List(ctx, &nodeList); err != nil {
		return err
	}

	findTaint := func(node corev1.Node) bool {
		for _, taint := range node.Spec.Taints {
			if isControllerTaint(taint) {
				return true
			}
		}
		return false
	}

	for _, node := range nodeList.Items {
		if !findTaint(node) {
			continue
		}
		if _, active := activeNodeNames[node.Name]; active {
			continue
		}

		var taints []corev1.Taint
		for _, taint := range node.Spec.Taints {
			if isControllerTaint(taint) {
				continue
			}
			taints = append(taints, taint)
		}
		node.Spec.Taints = taints

		if err := r.Update(ctx, &node); err != nil {
			return err
		}
	}

	return nil
}

func (r *NodeOperationReconciler) reconcilePending(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (ctrl.Result, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return ctrl.Result{}, err
	}

	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: "", Name: nodeOp.Spec.NodeName}, node); err != nil {
		return ctrl.Result{}, err
	}

	for _, taint := range node.Spec.Taints {
		if isControllerTaint(taint) {
			// This avoids multiple NodeOperations for the same Node to run simultaneously
			nodeOp.Status.Reason = "Another NodeOperation for the same node is running"
			return ctrl.Result{}, r.Update(ctx, nodeOp)
		}
	}

	violate, err := r.doesViolateNDB(ctx, nodeOp)
	if err != nil {
		return ctrl.Result{}, err
	}
	if violate {
		nodeOp.Status.Reason = "Due to NodeDisruptionBudget violation"
		if err := r.Update(ctx, nodeOp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: r.NDBRetryInterval}, nil
	}

	// Taint the node
	if err := r.taintNode(node); err != nil {
		return ctrl.Result{}, err
	}
	r.eventRecorder.Eventf(nodeOp, "Normal", "TaintNode", `Tainted a Node "%s"`, node.Name)

	nodeOp.Status.Reason = ""
	nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseDraining
	if err := r.Update(ctx, nodeOp); err != nil {
		return ctrl.Result{}, err
	}

	r.eventRecorder.Event(nodeOp, "Normal", "Draining", "Start to drain Pods")

	return ctrl.Result{}, nil
}

func (r *NodeOperationReconciler) reconcileDraining(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (ctrl.Result, error) {
	// Try to drain Pods
	drained, err := r.drain(ctx, nodeOp)
	if err != nil {
		return ctrl.Result{}, err
	}

	if nodeOp.Spec.SkipWaitingForEviction {
		nodeOp.Status.Reason = "WaitingForEvictionSkipped"
		nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseDrained
		if err := r.Update(ctx, nodeOp); err != nil {
			return ctrl.Result{}, err
		}

		r.eventRecorder.Event(nodeOp, "Normal", "Drained", "Skipped waiting for pods eviction")

		return ctrl.Result{}, nil
	}

	if !drained {
		return ctrl.Result{Requeue: true, RequeueAfter: r.DrainInterval}, nil
	}

	nodeOp.Status.Reason = ""
	nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseDrained
	if err := r.Update(ctx, nodeOp); err != nil {
		return ctrl.Result{}, err
	}

	r.eventRecorder.Event(nodeOp, "Normal", "Drained", "All Pods are drained")

	return ctrl.Result{}, nil
}

func (r *NodeOperationReconciler) reconcileDrained(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (ctrl.Result, error) {
	var childJobs batchv1.JobList
	if err := r.List(ctx, &childJobs, client.MatchingFields{jobOwnerKey: nodeOp.Name}); err != nil {
		return ctrl.Result{}, err
	}

	var job *batchv1.Job
	if len(childJobs.Items) == 0 {
		// Run a Job
		metadata := nodeOp.Spec.JobTemplate.Metadata.DeepCopy()
		if metadata.Name == "" && metadata.GenerateName == "" {
			metadata.GenerateName = fmt.Sprintf("nodeops-%s-", nodeOp.Name)
		}

		spec := nodeOp.Spec.JobTemplate.Spec.DeepCopy()
		if spec.Template.ObjectMeta.Annotations == nil {
			spec.Template.ObjectMeta.Annotations = map[string]string{}
		}
		spec.Template.ObjectMeta.Annotations["nodeops.k8s.preferred.jp/nodename"] = nodeOp.Spec.NodeName

		job = &batchv1.Job{
			ObjectMeta: *metadata,
			Spec:       *spec,
		}
		r.Log.Info("Creating a Job", "job", job)
		if err := ctrl.SetControllerReference(nodeOp, job, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, job); err != nil {
			return ctrl.Result{}, err
		}
		r.eventRecorder.Eventf(nodeOp, "Normal", "CreatedJob", `Created Job "%s" in "%s"`, job.Name, job.Namespace)
	} else if len(childJobs.Items) == 1 {
		job = &childJobs.Items[0]
	} else {
		nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseFailed
		nodeOp.Status.Reason = "more than 1 Job for this controller are found"
		if err := r.Update(ctx, nodeOp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	ref, err := reference.GetReference(r.Scheme, job)
	if err != nil {
		return ctrl.Result{}, err
	}
	nodeOp.Status.JobReference = *ref
	nodeOp.Status.Reason = ""
	nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseJobCreating
	if err := r.Update(ctx, nodeOp); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeOperationReconciler) reconcileJobCreating(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (ctrl.Result, error) {
	job := batchv1.Job{}
	ref := nodeOp.Status.JobReference
	if err := r.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &job); err != nil {
		sterr, ok := err.(*errors.StatusError)
		if ok && sterr.Status().Code == http.StatusNotFound {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseRunning
	if err := r.Update(ctx, nodeOp); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeOperationReconciler) reconcileRunning(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (ctrl.Result, error) {
	job := batchv1.Job{}
	ref := nodeOp.Status.JobReference

	if err := r.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, &job); err != nil {
		sterr, ok := err.(*errors.StatusError)
		if !ok || sterr.Status().Code != http.StatusNotFound {
			return ctrl.Result{}, err
		}
		// Job not found
		nodeOp.Status.Reason = "Job has been deleted"
		nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseFailed
	} else {
		_, condition := isJobFinished(&job)
		switch condition {
		case "": // ongoing
			return ctrl.Result{}, nil
		case batchv1.JobFailed:
			nodeOp.Status.Reason = "Job has failed"
			nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseFailed
		case batchv1.JobComplete:
			nodeOp.Status.Reason = "Job has completed"
			nodeOp.Status.Phase = nodeopsv1alpha1.NodeOperationPhaseCompleted
		}
		r.eventRecorder.Eventf(nodeOp, "Normal", "JobFinished", `Job "%s" in "%s" has finished`, job.Name, job.Namespace)
	}

	// untaint the Node
	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: "", Name: nodeOp.Spec.NodeName}, node); err != nil {
		return ctrl.Result{}, err
	}
	// After untainting, other NodeOperations for the node can proceed from Pending phase.
	if err := r.untaintNode(node); err != nil {
		return ctrl.Result{Requeue: true}, nil
	}
	r.eventRecorder.Eventf(nodeOp, "Normal", "UntaintNode", `Untainted a Node "%s"`, node.Name)

	// update after untaint
	if err := r.Update(ctx, nodeOp); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeOperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.clientset = kubernetes.NewForConfigOrDie(mgr.GetConfig())
	r.mutex = sync.Mutex{}
	// create index for NodeOperation name
	if err := mgr.GetFieldIndexer().IndexField(&batchv1.Job{}, jobOwnerKey, func(rawObj runtime.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != nodeopsv1alpha1.GroupVersion.String() || owner.Kind != "NodeOperation" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	r.eventRecorder = mgr.GetEventRecorderFor(eventSourceName)
	r.evictionStrategyProcessor = newEvictionStrategyProcessor(r, r.clientset, r.Log, r.eventRecorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeopsv1alpha1.NodeOperation{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// drain try to perform drain pods in the node of nodeOp and returns drained or not, or error.
func (r *NodeOperationReconciler) drain(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList); err != nil {
		return false, err
	}

	pods := []corev1.Pod{}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Spec.NodeName != nodeOp.Spec.NodeName {
			continue
		}
		if _, ok := pod.Annotations["kubernetes.io/config.mirror"]; ok { // mirror Pod
			continue
		}
		daemonSet := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "DaemonSet" {
				daemonSet = true
				break
			}
		}
		if daemonSet {
			continue
		}

		pods = append(pods, pod)
	}

	return r.evictionStrategyProcessor.do(pods, nodeOp)
}

func (r *NodeOperationReconciler) taintNode(node *corev1.Node) error {
	ctx := context.Background()
	for _, t := range node.Spec.Taints {
		if isControllerTaint(t) {
			return nil
		}
	}

	node.Spec.Taints = append(node.Spec.Taints, controllerTaint)
	if err := r.Update(ctx, node); err != nil {
		return err
	}
	return nil
}

func (r *NodeOperationReconciler) untaintNode(node *corev1.Node) error {
	ctx := context.Background()
	taints := []corev1.Taint{}
	for _, t := range node.Spec.Taints {
		if isControllerTaint(t) {
			continue
		}
		taints = append(taints, t)
	}
	node.Spec.Taints = taints
	if err := r.Update(ctx, node); err != nil {
		return err
	}
	return nil
}

func (r *NodeOperationReconciler) doesViolateNDB(ctx context.Context, nodeOp *nodeopsv1alpha1.NodeOperation) (bool, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return true, err
	}

	ndbList := &nodeopsv1alpha1.NodeDisruptionBudgetList{}
	if err := r.List(ctx, ndbList); err != nil {
		return true, err
	}

	return doesViolateNDB(nodeOp, ndbList.Items, nodeList.Items), nil
}

func doesViolateNDB(nodeOp *nodeopsv1alpha1.NodeOperation, ndbs []nodeopsv1alpha1.NodeDisruptionBudget, nodes []corev1.Node) bool {
	for _, ndb := range ndbs {
		if !labels.SelectorFromSet(nodeOp.Spec.NodeDisruptionBudgetSelector).Matches(labels.Set(ndb.Labels)) {
			continue
		}

		taintTargets := ndb.Spec.TaintTargets
		taintTargets = append(taintTargets, nodeopsv1alpha1.TaintTarget{
			Key:      controllerTaint.Key,
			Effect:   controllerTaint.Effect,
			Operator: nodeopsv1alpha1.TaintTargetOpExists,
		})

		var unavailableCount uint64
		selectedNodeNames := map[string]struct{}{}

	nextNode:
		for _, n := range nodes {
			for k, v := range ndb.Spec.Selector {
				if n.Labels[k] != v {
					continue nextNode
				}
			}
			selectedNodeNames[n.Name] = struct{}{}

		nextTaint:
			for _, taint := range n.Spec.Taints {
				isTarget := false
				for _, target := range taintTargets {
					if target.IsTarget(&taint) {
						isTarget = true
						break
					}
				}
				if !isTarget {
					continue nextTaint
				}

				unavailableCount++
				continue nextNode
			}
		}

		availableCount := uint64(len(selectedNodeNames)) - unavailableCount

		if ndb.Spec.MaxUnavailable != nil && *ndb.Spec.MaxUnavailable <= unavailableCount {
			return true
		}

		if ndb.Spec.MinAvailable != nil && availableCount <= *ndb.Spec.MinAvailable {
			return true
		}
	}

	return false
}

func isJobFinished(job *batchv1.Job) (bool, batchv1.JobConditionType) {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return true, c.Type
		}
	}

	return false, ""
}

func isControllerTaint(taint corev1.Taint) bool {
	return taint == controllerTaint
}
