package client

import (
	"context"
	"net"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/k8s"
)

// HostResolver is the narrow DNS dependency shared by network probes. Keeping
// it here lets application composition provide one resolver without coupling
// integrations to each other.
type HostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

// ClientSet contains the process-owned clients shared by Kwatch components.
// Components receive the narrow field they need; they must not construct a
// second client from configuration.
type ClientSet struct {
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
	REST       rest.Interface
	HTTP       *http.Client
	Resolver   HostResolver
}

// NewClientSetWithRuntime constructs clients from the immutable runtime
// snapshot used by application composition.
func NewClientSetWithRuntime(
	runtime config.RuntimeConfig,
) (ClientSet, error) {
	appConfig := runtime.Application()
	return newClientSet(appConfig)
}

// NewHTTPClientWithRuntime builds the shared outbound client from the
// immutable runtime snapshot. Command paths that only need HTTP still use the
// same application-owned construction boundary as the server.
func NewHTTPClientWithRuntime(runtime config.RuntimeConfig) *http.Client {
	appConfig := runtime.Application()
	return k8s.NewHTTPClient(appConfig)
}

func newClientSet(appConfig config.ApplicationRuntime) (ClientSet, error) {
	restConfig, err := getRestConfig(appConfig)
	if err != nil {
		return ClientSet{}, err
	}
	kubernetesClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	coreRESTConfig := rest.CopyConfig(restConfig)
	coreRESTConfig.GroupVersion = &corev1.SchemeGroupVersion
	coreRESTConfig.APIPath = "/api"
	coreRESTConfig.NegotiatedSerializer = scheme.Codecs
	restClient, err := rest.RESTClientFor(coreRESTConfig)
	if err != nil {
		return ClientSet{}, err
	}
	return ClientSet{
		Kubernetes: kubernetesClient,
		Dynamic:    dynamicClient,
		Discovery:  discoveryClient,
		REST:       restClient,
		HTTP:       k8s.NewHTTPClient(appConfig),
		Resolver:   &net.Resolver{},
	}, nil
}
