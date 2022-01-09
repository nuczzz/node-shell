package k8s

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

var nodeLister listerv1.NodeLister

// initNodeInformer init node informer
func initNodeInformer(stopChan <-chan struct{}, factory informers.SharedInformerFactory) error {
	nodeInformer := factory.Core().V1().Nodes().Informer()
	go nodeInformer.Run(stopChan)

	if !cache.WaitForCacheSync(stopChan, nodeInformer.HasSynced) {
		return errors.Errorf("node informer WaitForCacheSync expect true but got false")
	}
	klog.Info("node informer WaitForCacheSync success")

	nodeLister = factory.Core().V1().Nodes().Lister()
	return nil
}

// GetNode get node info from informer
func GetNode(name string) (*corev1.Node, error) {
	return nodeLister.Get(name)
}
