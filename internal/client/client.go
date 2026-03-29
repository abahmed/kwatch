package client

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/abahmed/kwatch/internal/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

// Create returns kubernetes client after initializing it with in-cluster, or
// out of cluster config
func Create(appConfig *config.App) kubernetes.Interface {
	client, err := CreateClient(appConfig)
	if err != nil {
		klog.ErrorS(err, "failed to create kubernetes client")
		os.Exit(1)
	}
	return client
}

// CreateClient returns kubernetes client or an error
func CreateClient(appConfig *config.App) (kubernetes.Interface, error) {
	// try to use in cluster config
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.InfoS("cannot get kubernetes in cluster config", "error", err)

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

	klog.V(4).InfoS("created kubernetes client successfully")

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

func GetNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return "kwatch"
	}
	return namespace
}
