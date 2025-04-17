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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeRemediationSpec defines the desired state of NodeRemediation.
type NodeRemediationSpec struct {
	NodeRemediationSpecTemplate `json:",inline"`
	NodeName                    string `json:"nodeName"`
}

type NodeRemediationSpecTemplate struct {
	Rule                      NodeRemediationRule `json:"rule"`
	NodeOperationTemplateName string              `json:"nodeOperationTemplateName"`
}

type NodeRemediationRule struct {
	Conditions []NodeConditionMatcher `json:"conditions"`
}

type NodeConditionMatcher struct {
	Type   corev1.NodeConditionType `json:"type"`
	Status corev1.ConditionStatus   `json:"status"`
}

// NodeRemediationStatus defines the observed state of NodeRemediation.
type NodeRemediationStatus struct {
	ActiveNodeOperation corev1.ObjectReference `json:"activeNodeOperation,omitempty"`
	// OperationsCount is num of NodeOperations executed by the NodeRemediation. Once the Node is remediated, this count will be reset to 0.
	OperationsCount int64 `json:"operationsCount"`
	// NodeStatus represents whether Node should be remediated or not.
	NodeStatus NodeStatus `json:"nodeStatus"`
}

type NodeStatus string

const (
	NodeStatusUnknown NodeStatus = ""
	NodeStatusOK      NodeStatus = "OK"
	NodeStatusBad     NodeStatus = "Bad"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// NodeRemediation is the Schema for the noderemediations API.
type NodeRemediation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeRemediationSpec   `json:"spec,omitempty"`
	Status NodeRemediationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeRemediationList contains a list of NodeRemediation.
type NodeRemediationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeRemediation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeRemediation{}, &NodeRemediationList{})
}

func (r *NodeRemediation) CompareNodeCondition(conditions []corev1.NodeCondition) NodeStatus {
matchersLoop:
	for _, matcher := range r.Spec.Rule.Conditions {
		for _, cond := range conditions {
			if cond.Type == matcher.Type {
				switch cond.Status {
				case matcher.Status:
					continue matchersLoop
				case corev1.ConditionUnknown:
					return NodeStatusUnknown
				}
			}
		}
		return NodeStatusOK
	}
	return NodeStatusBad
}
