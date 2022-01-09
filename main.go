package main

import (
	"flag"

	"github.com/gin-gonic/gin"
	"k8s.io/klog"
	"k8s.io/sample-controller/pkg/signals"

	"github.com/nuczzz/node-shell/k8s"
)

const httpServerAddr = ":8080"

var kubeConfig = flag.String("kubeconfig", "", "absolute path of kubeconfig file")

func main() {
	flag.Parse()
	klog.InitFlags(nil)

	klog.Infof("kubeconfig: %s", *kubeConfig)

	// init kubernetes client
	if err := k8s.InitClientSetAndInformer(signals.SetupSignalHandler(), *kubeConfig); err != nil {
		klog.Errorf("InitClientSetAndInformer error")
		return
	}

	// init gin http server
	engine := gin.New()
	engine.GET("/api/v1/node/:nodeName/shell", NodeShell)

	klog.Infof("http server run on: %s", httpServerAddr)
	klog.Fatal(engine.Run(httpServerAddr))
}
