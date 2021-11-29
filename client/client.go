package client

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Create returns kubernetes client after initializing it with in-cluster, or
// out of cluster config
func Create() kubernetes.Interface {
	// try to use in cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		logrus.Warnf("cannot get kubernetes in cluster config: %v", err)

		// try to use out of cluster config
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			home := homedir.HomeDir()
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logrus.Fatalf("cannot build kubernetes out of cluster config: %v", err)
		}
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Fatalf("cannot create kubernetes client: %v", err)
	}

	return clientset
}
