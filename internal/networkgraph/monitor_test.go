package networkgraph

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/context"
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

func assertGraphDependency(t *testing.T, graph *kwcontext.ResourceGraph, kind, namespace, name, want string) {
	t.Helper()
	for _, dependency := range graph.DependenciesOf(kind, namespace, name) {
		if dependency == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, graph.DependenciesOf(kind, namespace, name))
}
