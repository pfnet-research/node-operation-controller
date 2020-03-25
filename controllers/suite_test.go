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
	"github.com/google/go-cmp/cmp"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nodeopsv1alpha1 "github.com/pfnet-research/node-operation-controller/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{envtest.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	trueV := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "config", "crd", "bases")},
		UseExistingCluster: &trueV,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	mgr, err := manager.New(cfg, manager.Options{})

	err = (&NodeOperationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("NodeOperation"),
		Scheme: scheme.Scheme,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	err = (&NodeRemediationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("NodeRemediation"),
		Scheme: scheme.Scheme,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	err = (&NodeRemediationTemplateReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("NodeRemediationTemplate"),
		Scheme: scheme.Scheme,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	stop := make(chan struct{})
	go func() {
		err = mgr.Start(stop)
		Expect(err).ToNot(HaveOccurred())
	}()

	Eventually(func() bool {
		nodeList := &corev1.NodeList{}
		Expect(k8sClient.List(context.Background(), nodeList)).NotTo(HaveOccurred())
		for _, node := range nodeList.Items {
			for _, c := range node.Status.Conditions {
				if c.Type == corev1.NodeReady && c.Status == corev1.ConditionFalse {
					return false
				}
			}
		}
		return true
	}, time.Minute).Should(BeTrue())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})
	return fn, requests
}

func StartTestManager(mgr manager.Manager, g *GomegaWithT) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.Expect(mgr.Start(stop)).NotTo(HaveOccurred())
	}()
	return stop, wg
}

func createTestNodeOperation(name string, nodeName string) *nodeopsv1alpha1.NodeOperation {
	return &nodeopsv1alpha1.NodeOperation{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("test-%s", name),
		},
		Spec: nodeopsv1alpha1.NodeOperationSpec{
			NodeName: nodeName,
			NodeOperationSpecTemplate: nodeopsv1alpha1.NodeOperationSpecTemplate{
				JobTemplate: nodeopsv1alpha1.JobTemplateSpec{
					Metadata: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{
									Name:    "c",
									Image:   "busybox",
									Command: []string{"echo", "Hello"},
								}},
								RestartPolicy: corev1.RestartPolicyNever,
								Tolerations: []corev1.Toleration{
									{Key: controllerTaint.Key, Operator: corev1.TolerationOpExists},
								},
							},
						},
					},
				},
			},
		},
	}
}

func cleanupTestResources() {
	ctx := context.Background()

	Expect(k8sClient.DeleteAllOf(ctx, &nodeopsv1alpha1.NodeRemediationTemplate{})).NotTo(HaveOccurred())
	Expect(k8sClient.DeleteAllOf(ctx, &nodeopsv1alpha1.NodeRemediation{})).NotTo(HaveOccurred())
	Expect(k8sClient.DeleteAllOf(ctx, &nodeopsv1alpha1.NodeOperation{})).NotTo(HaveOccurred())
	Expect(k8sClient.DeleteAllOf(ctx, &nodeopsv1alpha1.NodeDisruptionBudget{})).NotTo(HaveOccurred())
	Expect(k8sClient.DeleteAllOf(ctx, &batchv1.Job{}, client.InNamespace("default"))).NotTo(HaveOccurred())
	Expect(k8sClient.DeleteAllOf(ctx, &corev1.Pod{}, client.InNamespace("default")), client.GracePeriodSeconds(0)).NotTo(HaveOccurred())

	Eventually(func() int {
		list := nodeopsv1alpha1.NodeRemediationTemplateList{}
		Expect(k8sClient.List(ctx, &list)).NotTo(HaveOccurred())
		return len(list.Items)
	}, 30).Should(Equal(0))

	Eventually(func() int {
		list := nodeopsv1alpha1.NodeRemediationList{}
		Expect(k8sClient.List(ctx, &list)).NotTo(HaveOccurred())
		return len(list.Items)
	}, 30).Should(Equal(0))

	Eventually(func() int {
		list := nodeopsv1alpha1.NodeOperationList{}
		Expect(k8sClient.List(ctx, &list)).NotTo(HaveOccurred())
		return len(list.Items)
	}, 30).Should(Equal(0))

	Eventually(func() int {
		list := nodeopsv1alpha1.NodeDisruptionBudgetList{}
		Expect(k8sClient.List(ctx, &list)).NotTo(HaveOccurred())
		return len(list.Items)
	}, 30).Should(Equal(0))

	Eventually(func() int {
		list := batchv1.JobList{}
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"))).NotTo(HaveOccurred())
		return len(list.Items)
	}, 30).Should(Equal(0))

	Eventually(func() int {
		list := corev1.PodList{}
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"))).NotTo(HaveOccurred())
		return len(list.Items)
	}, 60).Should(Equal(0))

	nodeList := corev1.NodeList{}
	Expect(k8sClient.List(ctx, &nodeList)).NotTo(HaveOccurred())
	for _, node := range nodeList.Items {
		var conditions []corev1.NodeCondition
		for _, c := range node.Status.Conditions {
			if strings.HasPrefix(string(c.Type), "Test") {
				continue
			}
			conditions = append(conditions, c)
		}
		node.Status.Conditions = conditions
		Expect(k8sClient.Status().Update(ctx, &node)).NotTo(HaveOccurred())
	}
}

var nodeNames = []string{
	"node-operation-controller-test-control-plane",
	"node-operation-controller-test-worker",
}

var eventuallyTimeout = time.Second * 20

var _ = BeforeEach(func() {
	cleanupTestResources()
})

var _ = Describe("NodeOperation", func() {
	Describe("taints a Node, create a Job and untaint the Node", func() {
		ctx := context.TODO()
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
					key, err := client.ObjectKeyFromObject(podToBeEvicted)
					Expect(err).To(BeNil())
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
				key, _ := client.ObjectKeyFromObject(op)
				k8sClient.Get(ctx, key, op)
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
		ctx := context.TODO()

		op1 := createTestNodeOperation("avoid-multi-ops-1", nodeNames[0])
		Expect(k8sClient.Create(ctx, op1)).NotTo(HaveOccurred())

		op2 := createTestNodeOperation("avoid-multi-ops-2", nodeNames[0])
		Expect(k8sClient.Create(ctx, op2)).NotTo(HaveOccurred())

		Eventually(func() []nodeopsv1alpha1.NodeOperationPhase {
			key1, _ := client.ObjectKeyFromObject(op1)
			key2, _ := client.ObjectKeyFromObject(op2)

			k8sClient.Get(ctx, key1, op1)
			k8sClient.Get(ctx, key2, op2)

			return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
		}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
			nodeopsv1alpha1.NodeOperationPhaseCompleted,
			nodeopsv1alpha1.NodeOperationPhasePending,
		}))
	})

	Context("when NodeOperation is deleted before completion", func() {
		It("remove a taint from the Node", func() {
			ctx := context.TODO()

			op := createTestNodeOperation("remove-taint-on-deletion", nodeNames[0])
			op.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "infinity"}
			Expect(k8sClient.Create(ctx, op)).NotTo(HaveOccurred())

			Eventually(func() nodeopsv1alpha1.NodeOperationPhase {
				key, _ := client.ObjectKeyFromObject(op)
				k8sClient.Get(ctx, key, op)
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
				ctx := context.TODO()

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
					key1, _ := client.ObjectKeyFromObject(op1)
					key2, _ := client.ObjectKeyFromObject(op2)

					k8sClient.Get(ctx, key1, op1)
					k8sClient.Get(ctx, key2, op2)

					return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
				}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
					nodeopsv1alpha1.NodeOperationPhaseCompleted,
					nodeopsv1alpha1.NodeOperationPhasePending,
				}))
			})
		})

		Context("maxUnavailable=1", func() {
			It("taints only one Node at a time", func() {
				ctx := context.TODO()

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
					key1, _ := client.ObjectKeyFromObject(op1)
					key2, _ := client.ObjectKeyFromObject(op2)

					k8sClient.Get(ctx, key1, op1)
					k8sClient.Get(ctx, key2, op2)

					return []nodeopsv1alpha1.NodeOperationPhase{op1.Status.Phase, op2.Status.Phase}
				}, eventuallyTimeout).Should(Equal([]nodeopsv1alpha1.NodeOperationPhase{
					nodeopsv1alpha1.NodeOperationPhaseCompleted,
					nodeopsv1alpha1.NodeOperationPhasePending,
				}))
			})
		})
	})
})

var _ = Describe("NodeRemediationTemplate", func() {
	It("creates NodeRemediation", func() {
		ctx := context.Background()
		nodeName := nodeNames[1]

		template := nodeopsv1alpha1.NodeRemediationTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-remediation-template-1",
			},
			Spec: nodeopsv1alpha1.NodeRemediationTemplateSpec{
				NodeSelector: map[string]string{
					"kubernetes.io/hostname": nodeName,
				},
				Template: nodeopsv1alpha1.NodeRemediationTemplateTemplateSpec{
					Metadata: metav1.ObjectMeta{
						Labels: map[string]string{
							"k1": "v1",
						},
					},
					Spec: nodeopsv1alpha1.NodeRemediationSpecTemplate{
						Rule: nodeopsv1alpha1.NodeRemediationRule{
							Conditions: []nodeopsv1alpha1.NodeConditionMatcher{
								{Type: "TestRemediation", Status: corev1.ConditionTrue},
							},
						},
						NodeOperationTemplateName: "template1",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &template)).NotTo(HaveOccurred())

		Eventually(func() bool {
			remediationList := nodeopsv1alpha1.NodeRemediationList{}
			Expect(k8sClient.List(ctx, &remediationList)).NotTo(HaveOccurred())

			for _, remediation := range remediationList.Items {
				var ownerOk bool
				for _, owner := range remediation.OwnerReferences {
					if owner.Kind == "NodeRemediationTemplate" && owner.Name == template.Name {
						ownerOk = true
						break
					}
				}

				var sameSpec bool
				if cmp.Equal(remediation.Spec.NodeRemediationSpecTemplate, template.Spec.Template.Spec) {
					sameSpec = true
				}

				var sameMeta bool
				if remediation.ObjectMeta.Labels["k1"] == "v1" {
					sameMeta = true
				}

				//fmt.Printf("%v %v %v\n", ownerOk, sameSpec, sameMeta)
				if ownerOk && sameSpec && sameMeta {
					return true
				}
			}

			return false
		}, eventuallyTimeout).Should(BeTrue())
	})
})

var _ = Describe("NodeRemediation", func() {
	It("creates NodeOperation", func() {
		ctx := context.Background()
		nodeName := nodeNames[1]

		template := nodeopsv1alpha1.NodeOperationTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-remediation-1",
			},
			Spec: nodeopsv1alpha1.NodeOperationTemplateSpec{
				Template: nodeopsv1alpha1.NodeOperationTemplateTemplateSpec{
					Spec: nodeopsv1alpha1.NodeOperationSpecTemplate{
						JobTemplate: nodeopsv1alpha1.JobTemplateSpec{
							Metadata: metav1.ObjectMeta{
								Namespace: "default",
							},
							Spec: batchv1.JobSpec{
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{{
											Name:    "c",
											Image:   "busybox",
											Command: []string{"echo", "Hello"},
										}},
										RestartPolicy: corev1.RestartPolicyNever,
										Tolerations: []corev1.Toleration{
											{Key: controllerTaint.Key, Operator: corev1.TolerationOpExists},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &template)).NotTo(HaveOccurred())

		remediation := nodeopsv1alpha1.NodeRemediation{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-remediation-1",
			},
			Spec: nodeopsv1alpha1.NodeRemediationSpec{
				NodeRemediationSpecTemplate: nodeopsv1alpha1.NodeRemediationSpecTemplate{
					Rule: nodeopsv1alpha1.NodeRemediationRule{
						Conditions: []nodeopsv1alpha1.NodeConditionMatcher{
							{Type: "TestRemediation", Status: corev1.ConditionTrue},
						},
					},
					NodeOperationTemplateName: template.Name,
				},
				NodeName: nodeName,
			},
		}
		Expect(k8sClient.Create(ctx, &remediation)).NotTo(HaveOccurred())

		node := corev1.Node{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, &node)).NotTo(HaveOccurred())

		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:               "TestRemediation",
			Status:             corev1.ConditionTrue,
			Reason:             "testing",
			Message:            "testing",
			LastHeartbeatTime:  metav1.NewTime(time.Now()),
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		Expect(k8sClient.Status().Update(ctx, &node)).NotTo(HaveOccurred())

		Eventually(func() bool {
			nodeOpList := nodeopsv1alpha1.NodeOperationList{}
			Expect(k8sClient.List(ctx, &nodeOpList)).NotTo(HaveOccurred())

			for _, op := range nodeOpList.Items {
				for _, owner := range op.OwnerReferences {
					if owner.Kind == "NodeRemediation" && owner.Name == remediation.Name {
						return true
					}
				}
			}

			return false
		}, eventuallyTimeout).Should(BeTrue())
	})
})
