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

package v1alpha1

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeOperationSpec defines the desired state of NodeOperation.
type NodeOperationSpec struct {
	NodeName                  string `json:"nodeName"`
	NodeOperationSpecTemplate `json:",inline"`
}

type NodeOperationSpecTemplate struct {
	// EvictionStrategy defines how to evict pods before performing the node operation.
	// The value must be one of Evict, Delete, ForceDelete, None (default=Evict)
	// TODO(everpeace): add default markers in the future for CRD
	//   ref: https://github.com/kubernetes-sigs/controller-tools/issues/250
	// +kubebuilder:validation:Enum=Evict;Delete;ForceDelete;None
	EvictionStrategy             NodeOperationEvictionStrategy `json:"evictionStrategy,omitempty"`
	SkipWaitingForEviction       bool                          `json:"skipWaitingForEviction,omitempty"`
	NodeDisruptionBudgetSelector map[string]string             `json:"nodeDisruptionBudgetSelector,omitempty"`
	JobTemplate                  JobTemplateSpec               `json:"jobTemplate"`
}

type NodeOperationEvictionStrategy string

const (
	NodeOperationEvictionStrategyEvict       NodeOperationEvictionStrategy = "Evict"
	NodeOperationEvictionStrategyDelete      NodeOperationEvictionStrategy = "Delete"
	NodeOperationEvictionStrategyForceDelete NodeOperationEvictionStrategy = "ForceDelete"
	NodeOperationEvictionStrategyNone        NodeOperationEvictionStrategy = "None"
)

type JobTemplateSpec struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     batchv1.JobSpec   `json:"spec"`
}

// NodeOperationStatus defines the observed state of NodeOperation.
type NodeOperationStatus struct {
	Phase        NodeOperationPhase     `json:"phase"`
	Reason       string                 `json:"reason"`
	JobNamespace string                 `json:"jobNamespace"` // Deprecated
	JobName      string                 `json:"jobName"`      // Deprecated
	JobReference corev1.ObjectReference `json:"jobReference,omitempty"`
}

type NodeOperationPhase string

const (
	NodeOperationPhasePending     NodeOperationPhase = "Pending"
	NodeOperationPhaseDraining    NodeOperationPhase = "Draining"
	NodeOperationPhaseDrained     NodeOperationPhase = "Drained"
	NodeOperationPhaseJobCreating NodeOperationPhase = "JobCreating"
	NodeOperationPhaseRunning     NodeOperationPhase = "Running"
	NodeOperationPhaseCompleted   NodeOperationPhase = "Completed"
	NodeOperationPhaseFailed      NodeOperationPhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="NodeName",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="JobNamespace",type=string,JSONPath=`.status.jobReference.namespace`,priority=1
// +kubebuilder:printcolumn:name="JobName",type=string,JSONPath=`.status.jobReference.name`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodeOperation is the Schema for the nodeoperations API.
type NodeOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeOperationSpec   `json:"spec,omitempty"`
	Status NodeOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeOperationList contains a list of NodeOperation.
type NodeOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeOperation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeOperation{}, &NodeOperationList{})
}

func (o *NodeOperation) NodeRemediationName() string {
	for _, owner := range o.OwnerReferences {
		if owner.APIVersion == GroupVersion.String() && owner.Kind == "NodeRemediation" {
			return owner.Name
		}
	}
	return ""
}
