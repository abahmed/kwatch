package client

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// NewKubernetesClient creates a Kubernetes client from cluster or local config.
func NewKubernetesClient(appConfig *config.App) (kubernetes.Interface, error) {
	return newKubernetesClient(appConfig)
}

// Create is retained for compatibility. New code should use
// NewKubernetesClient.
func Create(appConfig *config.App) (kubernetes.Interface, error) {
	return NewKubernetesClient(appConfig)
}

// CreateClient is retained for compatibility. New code should use
// NewKubernetesClient.
func CreateClient(appConfig *config.App) (kubernetes.Interface, error) {
	return NewKubernetesClient(appConfig)
}

func newKubernetesClient(appConfig *config.App) (kubernetes.Interface, error) {
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
		if p, err := url.Parse(appConfig.ProxyURL); err == nil {
			clientConfig.Proxy = http.ProxyURL(p)
		}
	}

	// Raise QPS/Burst from client-go defaults (5/10) to reduce throttling on large clusters
	clientConfig.QPS = 50
	clientConfig.Burst = 100

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

func GetRestConfig(appConfig *config.App) (*rest.Config, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.InfoS("cannot get kubernetes in cluster config", "error", err)
		kubeconfigPath := getKubeconfigPath()
		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("cannot build kubernetes out of cluster config: %w", err)
		}
	}
	if len(appConfig.ProxyURL) > 0 && clientConfig.Proxy == nil {
		if p, err := url.Parse(appConfig.ProxyURL); err == nil {
			clientConfig.Proxy = http.ProxyURL(p)
		}
	}
	return clientConfig, nil
}
