//go:build e2e

package harness

import (
	"fmt"
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Environment struct {
	Client    kubernetes.Interface
	Dynamic   dynamic.Interface
	Config    Config
	Health    *HealthClient
	Audit     *AuditReader
	Receiver  *ReceiverClient
	Artifacts *ArtifactWriter
	// diagnostics makes sure a scenario's state is captured once: the first
	// capture runs before the scenario namespace is deleted, and a later
	// one would overwrite it with a cluster that no longer has it.
	diagnostics sync.Once
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
	environment := &Environment{
		Client:  client,
		Dynamic: dynamicClient,
		Config:  config,
	}
	environment.Health = NewHealthClient(environment)
	environment.Audit = NewAuditReader(environment)
	environment.Receiver = NewReceiverClient(environment)
	environment.Artifacts, err = NewArtifactWriter(config.Artifacts)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

// Close releases what the environment holds open: the receiver
// port-forward.
func (e *Environment) Close() error {
	return e.Receiver.Close()
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
