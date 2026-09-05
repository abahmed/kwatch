package networkgraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
)

var watchedResources = []struct {
	gvr        schema.GroupVersionResource
	kind       string
	namespaced bool
}{
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "gatewayclasses",
		},
		"gatewayclass", false,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "gateways",
		},
		"gateway", true,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "httproutes",
		},
		"httproute", true,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "grpcroutes",
		},
		"grpcroute", true,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1alpha2",
			Resource: "tcproutes",
		},
		"tcproute", true,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1alpha2",
			Resource: "tlsroutes",
		},
		"tlsroute", true,
	},
	{
		schema.GroupVersionResource{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Resource: "referencegrants",
		},
		"referencegrant", true,
	},
}

type Monitor struct {
	client          dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	graph           *kwcontext.ResourceGraph
	resync          time.Duration
	allowed         func(string) bool
	namespaces      []string
	watchAll        bool
}

func New(restConfig *rest.Config, graph *kwcontext.ResourceGraph, resync time.Duration) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("networkgraph: create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("networkgraph: create discovery client: %w", err)
	}
	return &Monitor{
		client: client, discoveryClient: discoveryClient, graph: graph,
		resync: resync, watchAll: true,
	}, nil
}

// SetNamespaceFilter keeps graph edges from namespaced Gateway API objects
// within the same scope as the controller's typed informers.
func (m *Monitor) SetNamespaceFilter(filter func(string) bool) { m.allowed = filter }

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
}

func (m *Monitor) Start(ctx context.Context) error {
	factories := make([]dynamicinformer.DynamicSharedInformerFactory, 0)
	for _, watched := range watchedResources {
		if !m.resourceAvailable(watched.gvr) {
			continue
		}
		for _, namespace := range m.watchNamespaces(watched.namespaced) {
			factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
				m.client, m.resync, namespace, nil,
			)
			informer := factory.ForResource(watched.gvr).Informer()
			if err := informer.SetTransform(k8s.TrimManagedFields); err != nil {
				return fmt.Errorf(
					"networkgraph: set %s cache transform: %w",
					watched.gvr, err,
				)
			}
			kind := watched.kind
			if _, err := informer.AddEventHandler(
				cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) { m.rebuild(kind, obj) },
					UpdateFunc: func(_, obj interface{}) {
						m.rebuild(kind, obj)
					},
					DeleteFunc: func(obj interface{}) { m.remove(kind, obj) },
				},
			); err != nil {
				return fmt.Errorf(
					"networkgraph: register %s informer: %w",
					watched.gvr, err,
				)
			}
			factories = append(factories, factory)
		}
	}
	for _, factory := range factories {
		factory.Start(ctx.Done())
	}
	return nil
}

func (m *Monitor) watchNamespaces(namespaced bool) []string {
	if !namespaced || m.watchAll {
		return []string{""}
	}
	return append([]string(nil), m.namespaces...)
}

func (m *Monitor) resourceAvailable(gvr schema.GroupVersionResource) bool {
	if m.discoveryClient == nil {
		return true
	}
	resources, err := m.discoveryClient.ServerResourcesForGroupVersion(
		gvr.GroupVersion().String(),
	)
	if err != nil {
		return false
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return true
		}
	}
	return false
}

func (m *Monitor) rebuild(kind string, obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || m.graph == nil {
		return
	}
	if m.allowed != nil && u.GetNamespace() != "" && !m.allowed(u.GetNamespace()) {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, 8)
	if kind == "gateway" {
		if class, _, _ := unstructured.NestedString(u.Object, "spec", "gatewayClassName"); class != "" {
			targets = append(targets, kwcontext.EdgeTarget{Kind: "gatewayclass", Name: class, Type: "uses_class"})
		}
		targets = append(targets, secretReferences(u)...)
	} else if strings.HasSuffix(kind, "route") {
		targets = append(targets, routeParents(u)...)
		targets = append(targets, routeBackends(u)...)
	}
	ns := u.GetNamespace()
	if kind == "gatewayclass" {
		ns = ""
	}
	m.graph.ReplaceOutgoingEdges(kind, ns, u.GetName(), targets)
}

func secretReferences(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	listeners, _, _ := unstructured.NestedSlice(u.Object, "spec", "listeners")
	targets := make([]kwcontext.EdgeTarget, 0)
	for _, raw := range listeners {
		listener, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(listener, "tls", "certificateRefs")
		for _, rawRef := range refs {
			ref, ok := rawRef.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := ref["name"].(string)
			kind, _ := ref["kind"].(string)
			group, _ := ref["group"].(string)
			namespace, _ := ref["namespace"].(string)
			if namespace == "" {
				namespace = u.GetNamespace()
			}
			if name != "" &&
				(kind == "" || strings.EqualFold(kind, "Secret")) &&
				(group == "" || group == "core") {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "secret", Namespace: namespace, Name: name,
					Type: "tls_secret",
				})
			}
		}
	}
	return targets
}

func routeParents(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	parents, _, _ := unstructured.NestedSlice(u.Object, "spec", "parentRefs")
	targets := make([]kwcontext.EdgeTarget, 0, len(parents))
	for _, raw := range parents {
		ref, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := ref["name"].(string)
		kind, _ := ref["kind"].(string)
		ns, _ := ref["namespace"].(string)
		if ns == "" {
			ns = u.GetNamespace()
		}
		if name != "" && (kind == "" || strings.EqualFold(kind, "Gateway")) {
			targets = append(targets, kwcontext.EdgeTarget{Kind: "gateway", Namespace: ns, Name: name, Type: "routes_to"})
		}
	}
	return targets
}

func routeBackends(u *unstructured.Unstructured) []kwcontext.EdgeTarget {
	rules, _, _ := unstructured.NestedSlice(u.Object, "spec", "rules")
	targets := make([]kwcontext.EdgeTarget, 0)
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(rule, "backendRefs")
		for _, rawRef := range refs {
			ref, ok := rawRef.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := ref["name"].(string)
			kind, _ := ref["kind"].(string)
			group, _ := ref["group"].(string)
			namespace, _ := ref["namespace"].(string)
			if namespace == "" {
				namespace = u.GetNamespace()
			}
			if name != "" && (kind == "" || strings.EqualFold(kind, "Service")) && (group == "" || group == "core") {
				targets = append(targets, kwcontext.EdgeTarget{
					Kind: "service", Namespace: namespace, Name: name,
					Type: "routes_to",
				})
			}
		}
	}
	return targets
}

func (m *Monitor) remove(kind string, obj interface{}) {
	if m.graph == nil {
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}
	if kind == "gatewayclass" {
		ns = ""
	}
	m.graph.RemoveNode(kind, ns, name)
}
