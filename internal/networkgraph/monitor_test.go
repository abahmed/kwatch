package networkgraph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestRebuildHTTPRouteLinksGatewayAndService(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	route := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "api", "namespace": "apps"},
		"spec": map[string]interface{}{
			"parentRefs": []interface{}{map[string]interface{}{"name": "public"}},
			"rules": []interface{}{map[string]interface{}{
				"backendRefs": []interface{}{map[string]interface{}{"name": "api-svc"}},
			}},
		},
	}}

	monitor.rebuild("httproute", route)

	assertGraphDependency(t, graph, "httproute", "apps", "api", "gateway/apps/public")
	assertGraphDependency(t, graph, "httproute", "apps", "api", "service/apps/api-svc")
}

func TestRemoveHandlesDeletionTombstone(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	graph.AddEdge("gateway", "apps", "public", "service", "apps", "api", "routes_to")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "public", "namespace": "apps"},
	}}

	monitor.remove("gateway", cache.DeletedFinalStateUnknown{Key: "apps/public", Obj: obj})

	if got := graph.DependenciesOf("gateway", "apps", "public"); len(got) != 0 {
		t.Fatalf("tombstone delete left graph dependencies: %v", got)
	}
}

func TestRebuildPreservesCrossNamespaceReferences(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	route := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "route", "namespace": "frontend",
		},
		"spec": map[string]interface{}{
			"rules": []interface{}{map[string]interface{}{
				"backendRefs": []interface{}{map[string]interface{}{
					"name": "api", "namespace": "backend",
				}},
			}},
		},
	}}

	monitor.rebuild("httproute", route)

	deps := graph.DependenciesOf("httproute", "frontend", "route")
	if len(deps) != 1 || deps[0] != "service/backend/api" {
		t.Fatalf("unexpected route dependencies: %v", deps)
	}
}

func TestGatewayPreservesCertificateNamespace(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	gateway := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "public", "namespace": "gateway-system",
		},
		"spec": map[string]interface{}{
			"listeners": []interface{}{map[string]interface{}{
				"tls": map[string]interface{}{
					"certificateRefs": []interface{}{
						map[string]interface{}{
							"name": "wildcard", "namespace": "certs",
						},
					},
				},
			}},
		},
	}}

	monitor.rebuild("gateway", gateway)

	assertGraphDependency(
		t, graph, "gateway", "gateway-system", "public",
		"secret/certs/wildcard",
	)
}

func TestReferenceGrantKeepsNamespaceOnDelete(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	grant := &unstructured.Unstructured{}
	grant.SetName("allow-api")
	grant.SetNamespace("backend")
	graph.AddEdge(
		"referencegrant", "backend", "allow-api",
		"service", "backend", "api", "allows",
	)

	monitor.remove("referencegrant", grant)

	if deps := graph.DependenciesOf(
		"referencegrant", "backend", "allow-api",
	); len(deps) != 0 {
		t.Fatalf("reference grant node was not removed: %v", deps)
	}
}

func TestWatchNamespacesUsesExplicitScope(t *testing.T) {
	monitor := &Monitor{namespaces: []string{"one", "two"}}
	if got := monitor.watchNamespaces(false); len(got) != 1 || got[0] != "" {
		t.Fatalf("cluster resource scope changed: %v", got)
	}
	got := monitor.watchNamespaces(true)
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("namespaced resource scope changed: %v", got)
	}
}

func assertGraphDependency(t *testing.T, graph *kwcontext.ResourceGraph, kind, namespace, name, want string) {
	t.Helper()
	for _, dependency := range graph.DependenciesOf(kind, namespace, name) {
		if dependency == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, graph.DependenciesOf(kind, namespace, name))
}
