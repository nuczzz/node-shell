package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog"

	"github.com/nuczzz/node-shell/k8s"
)

var wsUpgrade = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// NodeShell node shell handler for gin http handler
func NodeShell(ctx *gin.Context) {
	traceID := uuid.NewString()
	nodeName := ctx.Param("nodeName")
	klog.Infof("[%s] NodeShell node name: %s", traceID, nodeName)

	// http upgrade to websocket
	wsConn, err := wsUpgrade.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		klog.Errorf("[%s] upgrade error: %s", traceID, err.Error())
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}
	defer wsConn.Close()

	// check whether node exist
	if _, err = k8s.GetNode(nodeName); err != nil {
		klog.Errorf("[%s] GetNode error", traceID, err.Error())

		// node not found error
		if k8serror.IsNotFound(err) {
			_ = wsConn.WriteMessage(websocket.TextMessage, []byte(errors.Errorf("node %s not found", nodeName).Error()))
			return
		}

		// server internal error
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(errors.Wrap(err, "GetNode error").Error()))
		return
	}

	pod := generateNodeShellPod(nodeName)
	_ = wsConn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("start create pod %s/%s...", pod.Namespace, pod.Name)))

	// create pod
	if _, err = k8s.CreatePod(pod.Namespace, pod); err != nil {
		klog.Errorf("[%s] CreatePod error: %s", traceID, err.Error())
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(errors.Wrap(err, "CreatePod error").Error()))
		return
	}
	defer cleanPod(pod.Namespace, pod.Name)

	// wait for node shell container running
	_ = wsConn.WriteMessage(websocket.TextMessage, []byte("create pod success and wait for pod running..."))
	if err = k8s.WaitForContainer(pod.Namespace, pod.Name, nodeShellContainerName); err != nil {
		klog.Errorf("[%s] WaitForContainer error: %s", traceID, err.Error())
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(errors.Wrap(err, "WaitForContainer error").Error()))
		return
	}

	_ = wsConn.WriteMessage(websocket.TextMessage, []byte("node shell pod running success"))

	// start exec pod container
	if err := k8s.ExecPodContainer(pod.Namespace, pod.Name, nodeShellContainerName, []string{"/bin/bash"}, newTerminalHandler(wsConn)); err != nil {
		klog.Errorf("[%s] ExecPodContainer error: %s", traceID, err.Error())
		_ = wsConn.WriteMessage(websocket.TextMessage, []byte(errors.Wrap(err, "ExecPodContainer error").Error()))
		return
	}
}

func cleanPod(namespace, name string) {
	if err := k8s.DeletePod(namespace, name); err != nil {
		klog.Errorf("DeletePod error")
	}
}

const (
	nodeShellNamespace        = "default"       // all node shell pod namespace is default
	nodeShellContainerName    = "node-shell"    // container name of node shell
	nodeShellContainerImage   = "alpine:latest" // container image of node shell
	nodeShellContainerCommand = "nsenter"       // set node shell container command to nsenter
)

func boolPtr(b bool) *bool {
	return &b
}

// generateNodeShellPod generate pod with node name
func generateNodeShellPod(nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(), // random pod name to avoid conflict
			Namespace: nodeShellNamespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName, // set node name for pod

			// use the host's IPC, PID, network namespace
			HostIPC:     true,
			HostPID:     true,
			HostNetwork: true,

			RestartPolicy: corev1.RestartPolicyNever, // we do not want pod restart when error happened

			Containers: []corev1.Container{
				{
					Name:            nodeShellContainerName,
					Image:           nodeShellContainerImage,
					ImagePullPolicy: corev1.PullIfNotPresent, // IfNotPresent maybe more better for such case

					SecurityContext: &corev1.SecurityContext{
						Privileged: boolPtr(true), // run container in privileged mode
					},

					Command: []string{nodeShellContainerCommand},
					Args: []string{
						"-t",    // target process pid
						"1",     // pid 1 process in node, it means init process
						"-m",    // enter mount namespace
						"-u",    // enter UTS namespace
						"-i",    // enter IPC namespace
						"-n",    // enter network namespace
						"sleep", // avoid container exit after nsenter command
						"1h",    // sleep 1h, and container will exit after 1h
					},
				},
			},
		},
	}
}

type terminalHandler struct {
	wsConn *websocket.Conn
	resize chan remotecommand.TerminalSize
}

var _ k8s.StreamHandler = &terminalHandler{}

func newTerminalHandler(wsConn *websocket.Conn) k8s.StreamHandler {
	return &terminalHandler{
		wsConn: wsConn,
		resize: make(chan remotecommand.TerminalSize),
	}
}

const endOfTransmission = "\u0004"

func (handler *terminalHandler) Read(p []byte) (int, error) {
	_, data, err := handler.wsConn.ReadMessage()
	if err != nil {
		return copy(p, endOfTransmission), err
	}

	return copy(p, data), nil
}

func (handler *terminalHandler) Write(p []byte) (int, error) {
	// TODO: for debug, we use TextMessage
	//if err := handler.wsConn.WriteMessage(websocket.BinaryMessage, p); err != nil {
	if err := handler.wsConn.WriteMessage(websocket.TextMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (handler *terminalHandler) Next() *remotecommand.TerminalSize {
	ret := <-handler.resize
	return &ret
}
