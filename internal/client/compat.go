package client

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

// NewClientSet is retained for embedded callers that still provide the
// YAML-facing application settings. Production composition must use
// NewClientSetWithRuntime.
func NewClientSet(appConfig *config.App) (ClientSet, error) {
	if appConfig == nil {
		appConfig = &config.App{}
	}
	runtime := config.RuntimeConfigFor(&config.Config{App: *appConfig})
	return NewClientSetWithRuntime(runtime)
}

// NewKubernetesClient creates a Kubernetes client from cluster or local
// configuration. It is a compatibility entry point; application composition
// should build the shared ClientSet once.
func NewKubernetesClient(
	appConfig *config.App,
) (kubernetes.Interface, error) {
	if appConfig == nil {
		appConfig = &config.App{}
	}
	runtime := config.RuntimeConfigFor(&config.Config{App: *appConfig})
	clientConfig, err := getRestConfig(runtime.Application())
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create kubernetes client: %w", err)
	}
	klog.V(4).InfoS("created kubernetes client successfully")
	return clientset, nil
}

// GetRestConfig is retained for embedded callers that provide YAML-facing
// application settings. Production composition uses ClientSet.
func GetRestConfig(appConfig *config.App) (*rest.Config, error) {
	if appConfig == nil {
		appConfig = &config.App{}
	}
	runtime := config.RuntimeConfigFor(&config.Config{App: *appConfig})
	return getRestConfig(runtime.Application())
}
