package client

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

func getKubeconfigPath() string {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := homedir.HomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	return kubeconfigPath
}

func getRestConfig(appConfig config.ApplicationRuntime) (*rest.Config, error) {
	clientConfig, err := loadRestConfig()
	if err != nil {
		return nil, err
	}
	applyApplicationRuntime(clientConfig, appConfig)
	return clientConfig, nil
}

func loadRestConfig() (*rest.Config, error) {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		klog.InfoS("cannot get kubernetes in cluster config", "error", err)
		kubeconfigPath := getKubeconfigPath()
		clientConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("cannot build kubernetes out of cluster config: %w", err)
		}
	}
	return clientConfig, nil
}

func applyApplicationRuntime(
	clientConfig *rest.Config,
	appConfig config.ApplicationRuntime,
) {
	if len(appConfig.ProxyURL) > 0 &&
		clientConfig.Proxy == nil {
		if p, err := url.Parse(appConfig.ProxyURL); err == nil {
			clientConfig.Proxy = http.ProxyURL(p)
		}
	}
	// Keep typed, dynamic, discovery, and CRD clients at the same throughput
	// settings. Otherwise only the typed client gets the large-cluster tuning.
	clientConfig.QPS = 50
	clientConfig.Burst = 100
}
