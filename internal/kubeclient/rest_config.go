package kubeclient

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

func getKubeconfigPath() string {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := homedir.HomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	return kubeconfigPath
}

// getRestConfig builds the Kubernetes API configuration. The application
// proxy is intentionally not applied: it is for outbound alert traffic, and
// routing API credentials through it would expose them to the proxy.
func getRestConfig() (*rest.Config, error) {
	clientConfig, err := loadRestConfig()
	if err != nil {
		return nil, err
	}
	// Keep typed, dynamic, discovery, and CRD clients at the same throughput
	// settings. Otherwise only the typed client gets the large-cluster tuning.
	clientConfig.QPS = 50
	clientConfig.Burst = 100
	return clientConfig, nil
}

// electionRestConfig copies base with a small dedicated rate limit for
// Lease traffic.
func electionRestConfig(base *rest.Config) *rest.Config {
	cfg := rest.CopyConfig(base)
	cfg.QPS = 5
	cfg.Burst = 10
	cfg.RateLimiter = nil
	return cfg
}

func loadRestConfig() (*rest.Config, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.InfoS("cannot get kubernetes in cluster config", "error", err)
		kubeconfigPath := getKubeconfigPath()
		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf(
				"cannot build kubernetes out of cluster config: %w", err,
			)
		}
	}
	return clientConfig, nil
}
