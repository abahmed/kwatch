package client

import (
	"net"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func newTestClientSet(appConfig *config.App) (ClientSet, error) {
	if appConfig == nil {
		appConfig = &config.App{}
	}
	runtime := config.RuntimeConfigFor(&config.Config{App: *appConfig})
	return NewClientSetWithRuntime(runtime, &net.Resolver{}, clock.RealClock{})
}

func newTestKubernetesClient(
	appConfig *config.App,
) (kubernetes.Interface, error) {
	clients, err := newTestClientSet(appConfig)
	if err != nil {
		return nil, err
	}
	return clients.Kubernetes, nil
}
