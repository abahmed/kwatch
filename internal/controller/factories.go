package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
)

func resolveNamespaces(cfg *config.Config, clientset kubernetes.Interface) ([]string, error) {
	if cfg.NamespaceSelector != "" {
		list, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
			LabelSelector: cfg.NamespaceSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("namespaceSelector list failed: %w", err)
		}
		ns := make([]string, 0, len(list.Items))
		for _, n := range list.Items {
			ns = append(ns, n.Name)
		}
		return ns, nil
	}
	return cfg.AllowedNamespaces, nil
}

func newFactories(client kubernetes.Interface, namespaces []string, resync time.Duration) (factorySet, []informers.SharedInformerFactory) {
	if len(namespaces) <= 1 {
		var opts []informers.SharedInformerOption
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		} else {
			// Exclude kube-system from non-control-plane informers to reduce
			// memory and network overhead. The control-plane monitor uses a
			// dedicated kube-system-scoped factory.
			opts = append(opts, informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "metadata.namespace!=kube-system"
			}))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		// Create a separate factory for cluster-scoped resources (Nodes,
		// MutatingWebhookConfigurations, ValidatingWebhookConfigurations) that
		// must NOT inherit the namespace field selector.
		clusterFactory := informers.NewSharedInformerFactoryWithOptions(client, resync)
		return factorySet{global: factory, clusterScoped: clusterFactory}, []informers.SharedInformerFactory{factory, clusterFactory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(namespaces))
	for _, ns := range namespaces {
		opts := []informers.SharedInformerOption{informers.WithNamespace(ns)}
		factories = append(factories, informers.NewSharedInformerFactoryWithOptions(client, resync, opts...))
	}
	return factorySet{perNamespace: factories}, factories
}

type factorySet struct {
	global        informers.SharedInformerFactory
	perNamespace  []informers.SharedInformerFactory
	clusterScoped informers.SharedInformerFactory
}
