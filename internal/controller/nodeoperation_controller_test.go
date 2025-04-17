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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
)

var _ = Describe("NodeOperation Controller", func() {
	Describe("taints a Node, create a Job and untaint the Node", func() {
		ctx := context.Background()
		nodeName := nodeNames[0]
		terminationGracePeriodSeconds := int64(0)
		podNamespaceToBeEvicted := "default"
		podNameToBeEvicted := "pod-to-be-evicted"

		runTest := func(op *nodeopsv1alpha1.NodeOperation, createPodToBeEvicted bool, assertEventsOnPodToBeEvicted func(types.UID, *corev1.EventList)) {
			var podToBeEvicted *corev1.Pod
			if createPodToBeEvicted {
				podToBeEvicted = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: podNamespaceToBeEvicted,
						Name:      podNameToBeEvicted,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    "busybox",
								Image:   "busybox",
								Command: []string{"sleep", "infinity"},
							},
						},
						NodeSelector: map[string]string{
							"kubernetes.io/hostname": nodeName,
						},
						RestartPolicy: corev1.RestartPolicyNever,
						Tolerations: []corev1.Toleration{
							{Operator: corev1.TolerationOpExists},
						},
						TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					},
				}
				Expect(k8sClient.Create(ctx, podToBeEvicted)).NotTo(HaveOccurred())
				Eventually(func() corev1.PodPhase {
					key := client.ObjectKeyFromObject(podToBeEvicted)
					Expect(k8sClient.Get(ctx, key, podToBeEvicted)).NotTo(HaveOccurred())

					return podToBeEvicted.Status.Phase
				}, eventuallyTimeout).Should(Equal(corev1.PodRunning))
			}

			// asserting eventually happens:
			//   NodeOperation created --> Node Tainted --> Job Completed --> Node UnTainted
			Expect(k8sClient.Create(ctx, op)).NotTo(HaveOccurred())

			checkNodeTainted := func() bool {
				node := &corev1.Node{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, node)).ToNot(HaveOccurred())
				for _, t := range node.Spec.Taints {
					if t == controllerTaint {
						return true
					}
				}
				return false
			}

			Eventually(checkNodeTainted, eventuallyTimeout).Should(BeTrue())

			getJob := func() *batchv1.Job {
				jobList := &batchv1.JobList{}
				Expect(k8sClient.List(ctx, jobList)).ToNot(HaveOccurred())
				for _, job := range jobList.Items {
					for _, owner := range job.OwnerReferences {
						if owner.Kind == "NodeOperation" && owner.Name == op.Name {
							return &job
						}
					}
				}
				return nil
			}

			Eventually(func() *batchv1.Job {
				return getJob()
			}, eventuallyTimeout).ShouldNot(BeNil())

			Expect(getJob().Spec.Template.ObjectMeta.Annotations["nodeops.k8s.preferred.jp/nodename"]).To(Equal(op.Spec.NodeName))

			Eventually(func() bool {
				job := getJob()
				for _, c := range job.Status.Conditions {
					if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
						return true
					}
				}
				return false
			}, eventuallyTimeout).Should(BeTrue())

			Eventually(func() nodeopsv1alpha1.NodeOperationPhase {
				key := client.ObjectKeyFromObject(op)
				_ = k8sClient.Get(ctx, key, op)
				return op.Status.Phase
			}, eventuallyTimeout).Should(Equal(nodeopsv1alpha1.NodeOperationPhaseCompleted))

			Eventually(checkNodeTainted, eventuallyTimeout).Should(BeFalse())

			if assertEventsOnPodToBeEvicted != nil {
				events := &corev1.EventList{}
				Expect(k8sClient.List(ctx, events)).NotTo(HaveOccurred())
				assertEventsOnPodToBeEvicted(podToBeEvicted.UID, events)
			}
		}

		eventFoundOnPodToBeEvicted := func(reason, message string) func(types.UID, *corev1.EventList) {
			return func(podUID types.UID, events *corev1.EventList) {
				eventFound := false
				for _, event := range events.Items {
					// TODO: check GVK
					if event.InvolvedObject.Namespace == podNamespaceToBeEvicted &&
						event.InvolvedObject.Name == podNameToBeEvicted &&
						event.InvolvedObject.UID == podUID &&
						event.Type == corev1.EventTypeNormal &&
						event.Reason == reason &&
						event.Message == message {
						eventFound = true
						break
					}
				}
				Expect(eventFound).To(BeTrue())
			}
		}

		BeforeEach(func() {
			Expect(k8sClient.DeleteAllOf(ctx, &nodeopsv1alpha1.NodeOperation{})).NotTo(HaveOccurred())
			Expect(k8sClient.DeleteAllOf(ctx, &corev1.Pod{}, client.InNamespace(podNamespaceToBeEvicted)))
		})

		Context("with no evictionStrategy", func() {
			It("should observe Evicted events on pods in the node (i.e. it should fall back to Evict evictionStrategy)", func() {
				op := createTestNodeOperation("simple", nodeName)
				runTest(op, true, eventFoundOnPodToBeEvicted("Evicted", `Node Operation "test-simple" evicted a Pod`))
			})
		})
		Context("with evictionStrategy=Evict", func() {
			It("observed Evicted events on pods in the nodes", func() {
				op := createTestNodeOperation("simple", nodeName)
				op.Spec.EvictionStrategy = nodeopsv1alpha1.NodeOperationEvictionStrategyEvict
				runTest(op, true, eventFoundOnPodToBeEvicted("Evicted", `Node Operation "test-simple" evicted a Pod`))
			})
		})
		Context("with evictionStrategy=Delete", func() {
			It("observed Deleted events on pods in the node", func() {
				op := createTestNodeOperation("simple", nodeName)
				op.Spec.EvictionStrategy = nodeopsv1alpha1.NodeOperationEvictionStrategyDelete
				runTest(op, true, eventFoundOnPodToBeEvicted("Deleted", `Node Operation "test-simple" deleted a Pod`))
			})
		})
		Context("with evictionStrategy=ForceDelete", func() {
			It("observed ForceDeleted events on pods in the node", func() {
				op := createTestNodeOperation("simple", nodeName)
				op.Spec.EvictionStrategy = nodeopsv1alpha1.NodeOperationEvictionStrategyForceDelete
				runTest(op, true, eventFoundOnPodToBeEvicted("ForceDeleted", `Node Operation "test-simple" force deleted a Pod`))
			})
		})
		Context("with evictionStrategy=None", func() {
			It("observed no events on pods in the node", func() {
				op := createTestNodeOperation("simple", nodeName)
				op.Spec.EvictionStrategy = nodeopsv1alpha1.NodeOperationEvictionStrategyNone
				runTest(op, false, nil)
			})
		})
		Context("with evictionStrategy=None and skipWaitingForEviction=true", func() {
			It("observed no events on pods in the node", func() {
				op := createTestNodeOperation("simple", nodeName)
				op.Spec.EvictionStrategy = nodeopsv1alpha1.NodeOperationEvictionStrategyNone
				op.Spec.SkipWaitingForEviction = true
				runTest(op, true, nil)
			})
		})
	})

	It("does not run multiple operations against the same node at a time", func() {
		ctx := context.Background()

		op1 := createTestNodeOperation("avoid-multi-ops-1", nodeNames[0])
		Expect(k8sClient.Create(ctx, op1)).NotTo(HaveOccurred())

		op2 := createTestNodeOperation("avoid-multi-ops-2", nodeNames[0])
		Expect(k8sClient.Create(ctx, op2)).NotTo(HaveOccurred())

		Eventually(func() []nodeopsv1alpha1.NodeOperationPhase {
			key1 := client.ObjectKeyFromObject(op1)
			key2 := client.ObjectKeyFromObject(op2)

			_ = k8sClient.Get(ctx, key1, op1)
			_ = k8sClient.Get(ctx, key2, op2)

			return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
		}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
			nodeopsv1alpha1.NodeOperationPhaseCompleted,
			nodeopsv1alpha1.NodeOperationPhasePending,
		}))
	})

	Context("when NodeOperation is deleted before completion", func() {
		It("remove a taint from the Node", func() {
			ctx := context.Background()

			op := createTestNodeOperation("remove-taint-on-deletion", nodeNames[0])
			op.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "infinity"}
			Expect(k8sClient.Create(ctx, op)).NotTo(HaveOccurred())

			Eventually(func() nodeopsv1alpha1.NodeOperationPhase {
				key := client.ObjectKeyFromObject(op)
				_ = k8sClient.Get(ctx, key, op)
				return op.Status.Phase
			}, eventuallyTimeout).Should(Equal(nodeopsv1alpha1.NodeOperationPhaseRunning))

			Expect(k8sClient.Delete(ctx, op)).NotTo(HaveOccurred())

			Eventually(func() bool {
				node := corev1.Node{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: op.Spec.NodeName}, &node)).NotTo(HaveOccurred())
				for _, taint := range node.Spec.Taints {
					if taint == controllerTaint {
						return false
					}
				}
				return true
			}, eventuallyTimeout).Should(BeTrue())
		})
	})

	Context("with PodDisruptionBudget", func() {
	})

	Context("with NodeDisruptionBudget", func() {
		Context("minAvailable=1", func() {
			It("taints only one Node at a time", func() {
				ctx := context.Background()

				minAvailable := uint64(1)
				ndb := &nodeopsv1alpha1.NodeDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{Name: "test-1"},
					Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
						Selector:     map[string]string{},
						MinAvailable: &minAvailable,
					},
				}
				Expect(k8sClient.Create(ctx, ndb)).NotTo(HaveOccurred())

				op1 := createTestNodeOperation("ndb-minavailable-1", nodeNames[0])
				Expect(k8sClient.Create(ctx, op1)).NotTo(HaveOccurred())

				op2 := createTestNodeOperation("ndb-minavailable-2", nodeNames[1])
				Expect(k8sClient.Create(ctx, op2)).NotTo(HaveOccurred())

				Eventually(func() []nodeopsv1alpha1.NodeOperationPhase {
					key1 := client.ObjectKeyFromObject(op1)
					key2 := client.ObjectKeyFromObject(op2)

					_ = k8sClient.Get(ctx, key1, op1)
					_ = k8sClient.Get(ctx, key2, op2)

					return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
				}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
					nodeopsv1alpha1.NodeOperationPhaseCompleted,
					nodeopsv1alpha1.NodeOperationPhasePending,
				}))
			})
		})

		Context("maxUnavailable=1", func() {
			It("taints only one Node at a time", func() {
				ctx := context.Background()

				maxUnavailable := uint64(1)
				ndb := &nodeopsv1alpha1.NodeDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{Name: "test-2"},
					Spec: nodeopsv1alpha1.NodeDisruptionBudgetSpec{
						Selector:       map[string]string{},
						MaxUnavailable: &maxUnavailable,
					},
				}
				Expect(k8sClient.Create(ctx, ndb)).NotTo(HaveOccurred())

				op1 := createTestNodeOperation("ndb-maxunavailable-1", nodeNames[0])
				Expect(k8sClient.Create(ctx, op1)).NotTo(HaveOccurred())

				op2 := createTestNodeOperation("ndb-maxunavailable-2", nodeNames[1])
				Expect(k8sClient.Create(ctx, op2)).NotTo(HaveOccurred())

				Eventually(func() []nodeopsv1alpha1.NodeOperationPhase {
					key1 := client.ObjectKeyFromObject(op1)
					key2 := client.ObjectKeyFromObject(op2)

					_ = k8sClient.Get(ctx, key1, op1)
					_ = k8sClient.Get(ctx, key2, op2)

					return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
				}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
					nodeopsv1alpha1.NodeOperationPhaseCompleted,
					nodeopsv1alpha1.NodeOperationPhasePending,
				}))
			})
		})
	})
})

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
