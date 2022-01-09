package k8s

import (
	"github.com/pkg/errors"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	clientSet    *kubernetes.Clientset
	clientConfig *rest.Config
)

// InitClientSetAndInformer init global client set and informer
func InitClientSetAndInformer(stopChan <-chan struct{}, kubeConfig string) error {
	var err error
	clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return errors.Wrap(err, "BuildConfigFromFlags error")
	}

	clientSet, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "NewForConfig error")
	}

	factory := informers.NewSharedInformerFactory(clientSet, 0)
	// init node informer
	if err = initNodeInformer(stopChan, factory); err != nil {
		return errors.Wrap(err, "initNodeInformer error")
	}

	return nil
}
