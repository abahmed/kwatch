package kubeclient

import (
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
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
	// Election has its own rate limiter so heavy lists and log fetches on
	// Kubernetes cannot delay Lease renewals past the renew deadline.
	Election kubernetes.Interface
	Dynamic  dynamic.Interface
	// Metadata lists and watches object metadata only, for kinds whose
	// spec and status kwatch does not need.
	Metadata  metadata.Interface
	Discovery discovery.DiscoveryInterfaceWithContext
	REST      rest.Interface
	HTTP      *http.Client
	// ProbeHTTP is the active probes' client: no outbound proxy, system
	// roots plus the cluster CA.
	ProbeHTTP *http.Client
	// Kubelet reads node stats and metrics directly from each kubelet.
	Kubelet  *KubeletClient
	Resolver HostResolver
	Clock    clock.Clock
	// restConfig is the API configuration the kubelet client derives its
	// credentials and CA from.
	restConfig *rest.Config
}

// NewClientSetWithRuntime constructs every client from the immutable
// runtime snapshot. It is NewClusterClientSet followed by WithRuntime.
func NewClientSetWithRuntime(
	runtime config.RuntimeConfig,
	resolver HostResolver,
	timeSource clock.Clock,
) (ClientSet, error) {
	clients, err := NewClusterClientSet(resolver, timeSource)
	if err != nil {
		return ClientSet{}, err
	}
	return clients.WithRuntime(runtime)
}

// NewClusterClientSet constructs the Kubernetes API clients, which do not
// depend on configuration. The application builds them first because the
// startup KwatchConfig overlay is read with the dynamic client; the
// configuration-dependent clients follow once with WithRuntime.
func NewClusterClientSet(
	resolver HostResolver, timeSource clock.Clock,
) (ClientSet, error) {
	if resolver == nil {
		return ClientSet{}, fmt.Errorf("client resolver is required")
	}
	if timeSource == nil {
		return ClientSet{}, fmt.Errorf("client clock is required")
	}
	restConfig, err := getRestConfig()
	if err != nil {
		return ClientSet{}, err
	}
	clients, err := newClusterClients(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	clients.Resolver = resolver
	clients.Clock = timeSource
	return clients, nil
}

// WithRuntime returns a copy with the clients that depend on
// configuration: the outbound HTTP client, the probe client and the
// kubelet client. Call it
// once, after every configuration overlay.
func (c ClientSet) WithRuntime(runtime config.RuntimeConfig) (
	ClientSet, error,
) {
	if c.restConfig == nil || c.Kubernetes == nil || c.Clock == nil {
		return ClientSet{}, fmt.Errorf("cluster clients are required")
	}
	appConfig := runtime.Application()
	kubeletClient, err := newKubeletClient(
		c.restConfig, c.Kubernetes, appConfig, c.Clock)
	if err != nil {
		return ClientSet{}, err
	}
	c.HTTP = NewHTTPClient(appConfig)
	c.ProbeHTTP = NewProbeHTTPClient(c.restConfig)
	c.Kubelet = kubeletClient
	return c, nil
}

// NewHTTPClientWithRuntime builds the shared outbound client from the
// immutable runtime snapshot. Command paths that only need HTTP still use the
// same application-owned construction boundary as the server.
func NewHTTPClientWithRuntime(runtime config.RuntimeConfig) *http.Client {
	appConfig := runtime.Application()
	return NewHTTPClient(appConfig)
}

func newClusterClients(restConfig *rest.Config) (ClientSet, error) {
	kubernetesClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	electionClient, err := kubernetes.NewForConfig(
		electionRestConfig(restConfig),
	)
	if err != nil {
		return ClientSet{}, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return ClientSet{}, err
	}
	metadataClient, err := metadata.NewForConfig(restConfig)
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
		Election:   electionClient,
		Dynamic:    dynamicClient,
		Metadata:   metadataClient,
		Discovery:  discoveryClient,
		REST:       restClient,
		restConfig: restConfig,
	}, nil
}

// newKubeletClient builds the direct kubelet client from the API server
// configuration and kubelet.insecureSkipVerify.
func newKubeletClient(
	restConfig *rest.Config, nodes kubernetes.Interface,
	appConfig config.ApplicationRuntime, timeSource clock.Clock,
) (*KubeletClient, error) {
	transport, err := NewKubeletTransport(
		restConfig, appConfig.KubeletInsecureSkipVerify)
	if err != nil {
		return nil, err
	}
	return NewKubeletClient(KubeletClientConfig{
		Nodes: nodes, Transport: transport, Now: timeSource.Now,
	})
}
