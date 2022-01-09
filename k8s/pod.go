package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreatePod create pod with client set
func CreatePod(namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	return clientSet.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
}

// DeletePod delete pod with client set
func DeletePod(namespace, name string) error {
	return clientSet.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}


