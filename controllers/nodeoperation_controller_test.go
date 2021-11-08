package controllers

import (
	"testing"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDoesViolateNDB(t *testing.T) {
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.False(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithMinAvailableViolation(t *testing.T) {
	minAvailable := uint64(1)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MinAvailable: &minAvailable,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.True(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithMinAvailableNoViolation(t *testing.T) {
	minAvailable := uint64(0)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MinAvailable: &minAvailable,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.False(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithMaxUnavailableViolation(t *testing.T) {
	n := uint64(0)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MaxUnavailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.True(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithMaxUnavailableNoViolation(t *testing.T) {
	n := uint64(1)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MaxUnavailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.False(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithNodeSelectorNoViolation(t *testing.T) {
	n := uint64(0)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				Selector: map[string]string{
					"k1": "v1",
				},
				MinAvailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v2",
				},
			},
		},
	}

	assert.False(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithNodeSelectorViolation(t *testing.T) {
	n := uint64(1)
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				Selector: map[string]string{
					"k1": "v1",
				},
				MinAvailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v2",
				},
			},
		},
	}

	assert.True(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithNDBSelectorViolation(t *testing.T) {
	n := uint64(1)
	nodeOp := &nodeopsv1alpha1.NodeOperation{
		Spec: nodeopsv1alpha1.NodeOperationSpec{
			NodeOperationSpecTemplate: nodeopsv1alpha1.NodeOperationSpecTemplate{
				NodeDisruptionBudgetSelector: map[string]string{
					"k1": "v1",
				},
			},
		},
	}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v1",
				},
			},
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MinAvailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.True(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithNDBSelectorNoViolation(t *testing.T) {
	n := uint64(1)
	nodeOp := &nodeopsv1alpha1.NodeOperation{
		Spec: nodeopsv1alpha1.NodeOperationSpec{
			NodeOperationSpecTemplate: nodeopsv1alpha1.NodeOperationSpecTemplate{
				NodeDisruptionBudgetSelector: map[string]string{
					"k1": "v1",
				},
			},
		},
	}
	ndbs := []nodeopsv1alpha1.NodeDisruptionBudget{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"k1": "v2",
				},
			},
			Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
				MinAvailable: &n,
			},
		},
	}
	nodes := []corev1.Node{
		{},
	}

	assert.False(t, doesViolateNDB(nodeOp, ndbs, nodes))
}

func TestDoesViolateNDBWithTaintTargets(t *testing.T) {
	nodeOp := &nodeopsv1alpha1.NodeOperation{}
	buildNDBs := func(n uint64) []nodeopsv1alpha1.NodeDisruptionBudget {
		return []nodeopsv1alpha1.NodeDisruptionBudget{
			{
				Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
					TaintTargets: []nodeopsv1alpha1.TaintTarget{
						{
							Key:      "k1",
							Effect:   corev1.TaintEffectNoSchedule,
							Operator: nodeopsv1alpha1.TaintTargetOpExists,
						},
					},
					MaxUnavailable: &n,
				},
			},
		}
	}
	nodes := []corev1.Node{
		{
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					controllerTaint,
				},
			},
		},
		{
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "k1",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		},
		{
			Spec: corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "k2",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
		},
	}

	assert.True(t, doesViolateNDB(nodeOp, buildNDBs(2), nodes))
	assert.False(t, doesViolateNDB(nodeOp, buildNDBs(3), nodes))
}
