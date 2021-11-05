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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeDisruptionBudgetSpec defines the desired state of NodeDisruptionBudget
type NodeDisruptionBudgetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Selector       map[string]string `json:"selector"`
	MaxUnavailable *uint64           `json:"maxUnavailable,omitempty"`
	MinAvailable   *uint64           `json:"minAvailable,omitempty"`
	// TaintTargets defines taints by which nodes are determined as unavailable. Default taints added by this controller are implicitly added to TaintTargets.
	TaintTargets []TaintTarget `json:"taintTargets,omitempty"`
}

type TaintTargetOperator string

const (
	TaintTargetOpExists TaintTargetOperator = "Exists"
	TaintTargetOpEqual  TaintTargetOperator = "Equal"
)

type TaintTarget struct {
	Key      string              `json:"key,omitempty"`
	Operator TaintTargetOperator `json:"operator,omitempty"`
	Value    string              `json:"value,omitempty"`
	Effect   corev1.TaintEffect  `json:"effect,omitempty"`
}

// NodeDisruptionBudgetStatus defines the observed state of NodeDisruptionBudget
type NodeDisruptionBudgetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="MaxUnavailable",type=integer,JSONPath=`.spec.maxUnavailable`
//+kubebuilder:printcolumn:name="MinAvailable",type=integer,JSONPath=`.spec.minAvailable`

// NodeDisruptionBudget is the Schema for the nodedisruptionbudgets API
type NodeDisruptionBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeDisruptionBudgetSpec   `json:"spec,omitempty"`
	Status NodeDisruptionBudgetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeDisruptionBudgetList contains a list of NodeDisruptionBudget
type NodeDisruptionBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeDisruptionBudget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeDisruptionBudget{}, &NodeDisruptionBudgetList{})
}

func (t *TaintTarget) IsTarget(taint *corev1.Taint) bool {
	if len(t.Effect) > 0 && t.Effect != taint.Effect {
		return false
	}

	if len(t.Key) > 0 && t.Key != taint.Key {
		return false
	}

	switch t.Operator {
	// empty operator means Equal
	case "", TaintTargetOpEqual:
		return t.Value == taint.Value
	case TaintTargetOpExists:
		return true
	default:
		return false
	}
}
