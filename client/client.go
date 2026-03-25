package client

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/abahmed/kwatch/config"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Create returns kubernetes client after initializing it with in-cluster, or
// out of cluster config
func Create(appConfig *config.App) kubernetes.Interface {
	client, err := CreateClient(appConfig)
	if err != nil {
		logrus.Fatalf("failed to create kubernetes client: %v", err)
	}
	return client
}

// CreateClient returns kubernetes client or an error
func CreateClient(appConfig *config.App) (kubernetes.Interface, error) {
	// try to use in cluster config
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		logrus.Warnf("cannot get kubernetes in cluster config: %v", err)

		// try to use out of cluster config
		kubeconfigPath := getKubeconfigPath()

		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("cannot build kubernetes out of cluster config: %w", err)
		}
	}

	// avoid using default app proxy if it's set
	if len(appConfig.ProxyURL) > 0 && clientConfig.Proxy == nil {
		clientConfig.Proxy = http.ProxyURL(nil)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create kubernetes client: %w", err)
	}

	logrus.Debugf("created kubernetes client successfully")

	return clientset, nil
}

func getKubeconfigPath() string {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := homedir.HomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	return kubeconfigPath
}
