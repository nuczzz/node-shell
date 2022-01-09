package k8s

import (
	"context"
	"io"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	watchtools "k8s.io/client-go/tools/watch"
)

const waitTimeoutSeconds = 60

// WaitForContainer watches the given pod util the container is running
func WaitForContainer(namespace, pod, container string) error {
	ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), waitTimeoutSeconds*time.Second)
	defer cancel()

	filedSelector := fields.OneTermEqualSelector("metadata.name", pod).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = filedSelector
			return clientSet.CoreV1().Pods(namespace).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = filedSelector
			return clientSet.CoreV1().Pods(namespace).Watch(ctx, options)
		},
	}

	_, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Deleted:
			return false, k8serror.NewNotFound(schema.GroupResource{Resource: "pods"}, pod)
		}

		p, ok := event.Object.(*corev1.Pod)
		if !ok {
			return false, errors.Errorf("event object is not pod")
		}

		status := getContainerStatusByName(p, container)
		if status == nil {
			return false, nil
		}

		if status.State.Running != nil || status.State.Terminated != nil {
			return true, nil
		}

		return false, nil
	})
	return err
}

func getContainerStatusByName(pod *corev1.Pod, container string) *corev1.ContainerStatus {
	allContainerStatus := [][]corev1.ContainerStatus{
		pod.Status.InitContainerStatuses,
		pod.Status.ContainerStatuses,
		pod.Status.EphemeralContainerStatuses,
	}
	for _, statusSlice := range allContainerStatus {
		for i := range statusSlice {
			if statusSlice[i].Name == container {
				return &statusSlice[i]
			}
		}
	}
	return nil
}

// StreamHandler stream handler definition for pod container exec
type StreamHandler interface {
	io.Reader
	io.Writer
	remotecommand.TerminalSizeQueue
}

// ExecPodContainer exec pod container like kubectl exec -ti {pod} -c {container} -- cmd
func ExecPodContainer(namespace, pod, container string, cmd []string, handler StreamHandler) error {
	req := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(clientConfig, "POST", req.URL())
	if err != nil {
		return errors.Wrap(err, "NewSPDYExecutor error")
	}

	return executor.Stream(remotecommand.StreamOptions{
		Stdin:             handler,
		Stdout:            handler,
		Stderr:            handler,
		Tty:               true,
		TerminalSizeQueue: handler,
	})
}
