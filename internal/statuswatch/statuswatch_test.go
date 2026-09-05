package statuswatch

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestFailureSignalUsesConfiguredConditionRules(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "db"},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Healthy", "status": "False", "reason": "ReplicaLag"},
		}},
	}}
	rules := map[string]map[string]bool{"Healthy": {"False": true}}
	if signal := failureSignal(obj, "customresource", "db", rules); signal == nil || signal.Hint != "Healthy=False: ReplicaLag" {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if signal := failureSignal(obj, "customresource", "db", defaultConditionRules()); signal != nil {
		t.Fatalf("default rules should ignore custom Healthy condition: %+v", signal)
	}
}

func TestReconcileCRDVersionsStopsUnservedVersion(t *testing.T) {
	stopped := false
	monitor := &Monitor{
		factories:   map[string]dynamicinformer.DynamicSharedInformerFactory{"old": nil},
		stops:       map[string]context.CancelFunc{"old": func() { stopped = true }},
		crdVersions: map[string]map[string]struct{}{"dbs.example.io": {"old": {}}},
	}

	monitor.reconcileCRDVersions("dbs.example.io", map[string]struct{}{"new": {}})

	if !stopped {
		t.Fatal("expected unserved CRD version to be stopped")
	}
	if _, exists := monitor.factories["old"]; exists {
		t.Fatal("stale CRD informer was not removed")
	}
}

func TestRebuildGraphTracksCustomResourceOwner(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "apps",
			"ownerReferences": []interface{}{map[string]interface{}{
				"kind": "Database", "name": "db-prod",
			}},
		},
	}}

	monitor.rebuildGraph(obj)

	if got := graph.DependenciesOf("customresource", "apps", "db"); len(got) != 1 || got[0] != "database/apps/db-prod" {
		t.Fatalf("unexpected custom resource dependencies: %v", got)
	}
}

func TestGraphReferenceRulesTraverseArrays(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	monitor := &Monitor{graph: graph}
	monitor.SetGraphReferenceRules([]string{"spec.backendRefs.name=service"})
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "route", "namespace": "apps"},
		"spec": map[string]interface{}{"backendRefs": []interface{}{
			map[string]interface{}{"name": "api"},
			map[string]interface{}{"name": "web"},
		}},
	}}

	monitor.rebuildGraph(obj)

	deps := graph.DependenciesOf("customresource", "apps", "route")
	if len(deps) != 2 || deps[0] != "service/apps/api" || deps[1] != "service/apps/web" {
		t.Fatalf("unexpected reference dependencies: %v", deps)
	}
}

func TestWatchNamespacesUsesExplicitScope(t *testing.T) {
	monitor := &Monitor{namespaces: []string{"apps"}}
	if got := monitor.watchNamespaces(false); len(got) != 1 || got[0] != "" {
		t.Fatalf("cluster resource scope changed: %v", got)
	}
	got := monitor.watchNamespaces(true)
	if len(got) != 1 || got[0] != "apps" {
		t.Fatalf("namespaced resource scope changed: %v", got)
	}
}

func TestCanWatchVersionSkipsForbiddenResource(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "WidgetList",
		},
	)
	client.PrependReactor(
		"list", "widgets",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(
				gvr.GroupResource(), "", errors.New("denied"),
			)
		},
	)
	monitor := &Monitor{client: client, ctx: context.Background()}

	if monitor.canWatchVersion(gvr, "apps") {
		t.Fatal("forbidden custom resource should not start an informer")
	}
}
