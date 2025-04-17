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

// NodeRemediationTemplateSpec defines the desired state of NodeRemediationTemplate.
type NodeRemediationTemplateSpec struct {
	NodeSelector map[string]string                   `json:"nodeSelector"`
	Template     NodeRemediationTemplateTemplateSpec `json:"template"`
}

type NodeRemediationTemplateTemplateSpec struct {
	// +kubebuilder:pruning:PreserveUnknownFields
	Metadata metav1.ObjectMeta           `json:"metadata,omitempty"`
	Spec     NodeRemediationSpecTemplate `json:"spec"`
}

// NodeRemediationTemplateStatus defines the observed state of NodeRemediationTemplate.
type NodeRemediationTemplateStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster

// NodeRemediationTemplate is the Schema for the noderemediationtemplates API.
type NodeRemediationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeRemediationTemplateSpec   `json:"spec,omitempty"`
	Status NodeRemediationTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeRemediationTemplateList contains a list of NodeRemediationTemplate.
type NodeRemediationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeRemediationTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeRemediationTemplate{}, &NodeRemediationTemplateList{})
}
