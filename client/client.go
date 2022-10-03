package client

import (
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
	// try to use in cluster config
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		logrus.Warnf("cannot get kubernetes in cluster config: %v", err)

		// try to use out of cluster config
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			home := homedir.HomeDir()
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}

		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logrus.Fatalf(
				"cannot build kubernetes out of cluster config: %v",
				err)
		}
	}

	// avoid using default app proxy if it's set
	if len(appConfig.ProxyURL) > 0 && clientConfig.Proxy == nil {
		clientConfig.Proxy = http.ProxyURL(nil)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		logrus.Fatalf("cannot create kubernetes client: %v", err)
	}

	logrus.Debugf("created kubernetes client successfully")

	return clientset
}
