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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
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

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = nodeopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    ptr.To(true),
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&NodeOperationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: scheme.Scheme,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&NodeRemediationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: scheme.Scheme,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&NodeRemediationTemplateReconciler{
		Client: k8sManager.GetClient(),
		Scheme: scheme.Scheme,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
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
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
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

var eventuallyTimeout = time.Second * 60

var _ = BeforeEach(func() {
	cleanupTestResources()
})
