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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeOperationTemplateSpec defines the desired state of NodeOperationTemplate.
type NodeOperationTemplateSpec struct {
	Template NodeOperationTemplateTemplateSpec `json:"template"`
}

type NodeOperationTemplateTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	Metadata metav1.ObjectMeta         `json:"metadata"`
	Spec     NodeOperationSpecTemplate `json:"spec"`
}

// NodeOperationTemplateStatus defines the observed state of NodeOperationTemplate.
type NodeOperationTemplateStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// NodeOperationTemplate is the Schema for the nodeoperationtemplates API.
type NodeOperationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeOperationTemplateSpec   `json:"spec,omitempty"`
	Status NodeOperationTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeOperationTemplateList contains a list of NodeOperationTemplate.
type NodeOperationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeOperationTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeOperationTemplate{}, &NodeOperationTemplateList{})
}
