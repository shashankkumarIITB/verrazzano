// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package pkg

import (
	"fmt"
	"strings"

	"github.com/verrazzano/verrazzano/pkg/k8sutil"
)

func PodsHaveAnnotation(namespace string, annotation string) bool {
	clientset, err := k8sutil.GetKubernetesClientset()
	if err != nil {
		Log(Error, fmt.Sprintf("Error getting clientset, error: %v", err))
		return false
	}
	pods, err := ListPodsInCluster(namespace, clientset)
	if err != nil {
		Log(Error, fmt.Sprintf("Error listing pods in cluster for namespace: %s, error: %v", namespace, err))
		return false
	}
	for _, pod := range pods.Items {
		_, hasAnnotation := pod.Annotations[annotation]
		if !hasAnnotation &&
			!strings.Contains(pod.Name, "vmi-system-kiali") &&
			!strings.Contains(pod.Name, "vmi-system-es-data") {
			return false
		}
	}
	return true
}

// CheckPodsForEnvoySidecar checks if a pods which have Envoy sidecars, have the specified image
func CheckPodsForEnvoySidecar(namespace string, imageName string) bool {
	clientset, err := k8sutil.GetKubernetesClientset()
	if err != nil {
		Log(Error, fmt.Sprintf("Error getting clientset, error: %v", err))
		return false
	}
	pods, err := ListPodsInCluster(namespace, clientset)
	if err != nil {
		Log(Error, fmt.Sprintf("Error listing pods in cluster for namespace: %s, error: %v", namespace, err))
		return false
	}
	if len(pods.Items) == 0 {
		Log(Info, fmt.Sprintf("No pods in namespace: %s, error: %v", namespace, err))
		return false
	}
	// Every pod with istio enabled must containe the Envoy sidecar
	for _, pod := range pods.Items {
		// skip if istio sidecar disabled
		v := pod.Labels["sidecar.istio.io/inject"]
		if v == "false" {
			continue
		}
		_, ok := pod.Labels["istio.io/rev"]
		if ok {
			containers := pod.Spec.Containers
			found := false
			for _, container := range containers {
				if strings.Contains(container.Image, imageName) {
					found = true
					break
				}
			}
			if !found {
				Log(Error, fmt.Sprintf("No istio proxy image found in pod %s", pod.Name))
				return false
			}
		}
	}
	return true
}