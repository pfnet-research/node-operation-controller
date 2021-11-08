package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()
	nodename := flag.String("nodename", "", "")
	addConditionsFlag := flag.String("add-conditions", "[]", "")
	removeConditionsFlag := flag.String("remove-condition-types", "", "comma separated")
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, *nodename, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	var addConditions []corev1.NodeCondition
	if err := json.Unmarshal([]byte(*addConditionsFlag), &addConditions); err != nil {
		panic(err.Error())
	}

	node.Status.Conditions = append(node.Status.Conditions, addConditions...)
	node.Status.Conditions = removeConditions(node.Status.Conditions, strings.Split(*removeConditionsFlag, ","))

	if _, err := clientset.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
		panic(err.Error())
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func removeConditions(conditions []corev1.NodeCondition, strtypes []string) []corev1.NodeCondition {
	types := make([]corev1.NodeConditionType, len(strtypes))
	for i, t := range strtypes {
		types[i] = corev1.NodeConditionType(t)
	}

	var filtered []corev1.NodeCondition
nextCondition:
	for _, c := range conditions {
		for _, t := range types {
			if c.Type == t {
				continue nextCondition
			}
		}
		filtered = append(filtered, c)
	}
	return filtered
}
