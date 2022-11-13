package server

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/labels"
	"os"
	"time"

	appsv1model "k8s.io/api/apps/v1"
	corev1model "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appsv1cli "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1cli "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Serve(ctx context.Context, clientset *kubernetes.Clientset) error {
	deployClient := clientset.AppsV1().Deployments("")
	podClients := make(map[string]corev1cli.PodInterface)
	nodeClient := clientset.CoreV1().Nodes()

	fmt.Println("[INFO ] --- current implementation reconcile once")

	nodeList, err := nodeClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		// TODO continue reconcile
		_, _ = fmt.Fprintln(os.Stderr, "[ERROR] --- failed to list Node")
		return err
	}
	var nodes map[string]*corev1model.Node
	for _, node := range nodeList.Items {
		nodes[node.Name] = node.DeepCopy()
	}

	fmt.Println("[INFO ] --- current implementation only treats Deployment")

	var deployList *appsv1model.DeploymentList
	deployList, err = deployClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		// TODO continue reconcile
		_, _ = fmt.Fprintln(os.Stderr, "[ERROR] --- failed to list Deployment")
		return err
	}

	for _, deploy := range deployList.Items {
		ns := deploy.Namespace
		podClient, found := podClients[ns]
		if !found {
			podClient = clientset.CoreV1().Pods(ns)
			podClients[ns] = podClient
		}

		podSelector := deploy.Spec.Selector

		var pods *corev1model.PodList
		pods, err = podClient.List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(podSelector),
		})
		if err != nil {
			fmt.Printf("[WARN ] --- failed to list pods: labelSelector: %s\n", podSelector.String())
			continue
		}

		willRestarts := false
		for _, pod := range pods.Items {
			node := nodes[pod.Spec.NodeName]
			affinity := pod.Spec.Affinity
			if affinity == nil {
				continue
			}
			podAntiAffinities := affinity.PodAntiAffinity
			if podAntiAffinities == nil {
				continue
			}

			fmt.Println("[INFO ] --- current implementation only check affinity rules \"requiredDuringSchedulingIgnoredDuringExecution\"s")
			for _, antiAffinity := range podAntiAffinities.RequiredDuringSchedulingIgnoredDuringExecution {
				for _, otherPod := range pods.Items {
					if pod.Name == otherPod.Name {
						continue
					}
					var selector labels.Selector
					selector, err = metav1.LabelSelectorAsSelector(antiAffinity.LabelSelector)
					if selector.Matches((labels.Set)(otherPod.Labels)) {
						otherNode := nodes[otherPod.Spec.NodeName]
						if node.Labels[antiAffinity.TopologyKey] == otherNode.Labels[antiAffinity.TopologyKey] {
							willRestarts = true
							break
						}
					}
				}
				// when a pod violating anti-affinity found, no other pods need to check
				if willRestarts {
					break
				}
			}
			// when a pod violating anti-affinity found, no other affinity-rules need to check
			if willRestarts {
				break
			}
		}

		if willRestarts {
			fmt.Printf("[INFO ] --- restarting: {Kind: Deployment, Namespace: %s, Name: %s}\n")
			err = restart(ctx, deployClient, deploy)
			if err != nil {
				fmt.Printf("[WARN ] --- failed to restart: {Kind: Deployment, Namespace: %s, Name: %s}: %#v\n", deploy.Namespace, deploy.Name, err)
				continue
			}
		}

	}

	return fmt.Errorf("currently")
}

func restart(ctx context.Context, client appsv1cli.DeploymentInterface, deployment appsv1model.Deployment) error {
	deployment.Spec.Template.Annotations["fargate-descheduler.10h.in/restartedAt"] = time.Now().Format("2002-01-02T15-04-05Z07:00")
	_, err := client.Update(ctx, &deployment, metav1.UpdateOptions{})
	return err
}
