//go:build e2e

package harness

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Environment struct {
	Client      kubernetes.Interface
	Dynamic     dynamic.Interface
	Discovery   discovery.DiscoveryInterface
	Config      Config
	Health      *HealthClient
	Diagnostics *DiagnosticsClient
	Audit       *AuditReader
	Receiver    *ReceiverClient
	Artifacts   *ArtifactWriter
}

func NewEnvironment(config Config) (*Environment, error) {
	restConfig, err := loadRESTConfig(config)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	environment := &Environment{
		Client:    client,
		Dynamic:   dynamicClient,
		Discovery: discoveryClient,
		Config:    config,
	}
	environment.Health = NewHealthClient(environment)
	environment.Diagnostics = NewDiagnosticsClient(environment)
	environment.Audit = NewAuditReader(environment)
	environment.Receiver = NewReceiverClient(environment)
	environment.Artifacts, err = NewArtifactWriter(config.Artifacts)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func loadRESTConfig(config Config) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if config.Kubeconfig != "" {
		rules.ExplicitPath = config.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if config.Context != "" {
		overrides.CurrentContext = config.Context
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules,
		overrides,
	).ClientConfig()
}

func (e *Environment) Lease(namespace, name string) (string, error) {
	lease, err := e.Client.CoordinationV1().Leases(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	if lease.Spec.HolderIdentity == nil {
		return "", nil
	}
	return string(*lease.Spec.HolderIdentity), nil
}
