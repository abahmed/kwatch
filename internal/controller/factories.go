package controller

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
)

type namespaceScope struct {
	namespaces []string
	all        bool
	forbidden  []string
}

var namespaceResolveTimeout = 30 * time.Second

func resolveNamespaces(cfg *config.Config, clientset kubernetes.Interface) (namespaceScope, error) {
	if cfg.NamespaceSelector != "" {
		ctx, cancel := context.WithTimeout(context.Background(), namespaceResolveTimeout)
		defer cancel()
		list, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
			LabelSelector: cfg.NamespaceSelector,
		})
		if err != nil {
			return namespaceScope{}, fmt.Errorf("namespaceSelector list failed: %w", err)
		}
		ns := make([]string, 0, len(list.Items))
		for _, n := range list.Items {
			ns = append(ns, n.Name)
		}
		return namespaceScope{namespaces: ns}, nil
	}
	return namespaceScope{
		namespaces: cfg.AllowedNamespaces,
		all:        len(cfg.AllowedNamespaces) == 0,
		forbidden:  cfg.ForbiddenNamespaces,
	}, nil
}

func newFactories(
	client kubernetes.Interface,
	scope namespaceScope,
	forbiddenNamespaces []string,
	resync time.Duration,
) (factorySet, []informers.SharedInformerFactory) {
	if scope.all || len(scope.namespaces) == 1 {
		var opts []informers.SharedInformerOption
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		} else {
			// Exclude configured namespaces, plus kube-system from non-control-plane
			// informers. The control-plane monitor uses a dedicated kube-system
			// scoped factory.
			excluded := informerExcludedNamespaces(forbiddenNamespaces)
			opts = append(opts, informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = excluded
			}))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		// Create a separate factory for cluster-scoped resources (Nodes,
		// MutatingWebhookConfigurations, ValidatingWebhookConfigurations) that
		// must NOT inherit the namespace field selector.
		clusterFactory := informers.NewSharedInformerFactoryWithOptions(client, resync)
		return factorySet{global: factory, clusterScoped: clusterFactory}, []informers.SharedInformerFactory{factory, clusterFactory}
	}

	if len(scope.namespaces) == 0 {
		clusterFactory := informers.NewSharedInformerFactoryWithOptions(client, resync)
		return factorySet{clusterScoped: clusterFactory}, []informers.SharedInformerFactory{clusterFactory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(scope.namespaces))
	for _, ns := range scope.namespaces {
		opts := []informers.SharedInformerOption{informers.WithNamespace(ns)}
		factories = append(factories, informers.NewSharedInformerFactoryWithOptions(client, resync, opts...))
	}
	return factorySet{perNamespace: factories}, factories
}

func informerExcludedNamespaces(forbidden []string) string {
	seenCap := 1
	if len(forbidden) < math.MaxInt {
		seenCap = len(forbidden) + 1
	}
	seen := make(map[string]struct{}, seenCap)
	seen["kube-system"] = struct{}{}
	for _, namespace := range forbidden {
		if namespace != "" {
			seen[namespace] = struct{}{}
		}
	}
	namespaces := make([]string, 0, len(seen))
	for namespace := range seen {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	selectors := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		selectors = append(selectors, "metadata.namespace!="+namespace)
	}
	return strings.Join(selectors, ",")
}

type factorySet struct {
	global        informers.SharedInformerFactory
	perNamespace  []informers.SharedInformerFactory
	clusterScoped informers.SharedInformerFactory
}
