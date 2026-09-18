package statuswatch

import (
	"context"
	"errors"
	"sync"
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
	signal := failureSignal(obj, "customresource", rules)
	if signal == nil || signal.Hint != "Healthy=False: ReplicaLag" {
		t.Fatalf("unexpected signal: %+v", signal)
	}
	if signal := failureSignal(
		obj, "customresource", defaultConditionRules(),
	); signal != nil {
		t.Fatalf(
			"default rules should ignore custom Healthy condition: %+v",
			signal,
		)
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
	if err := monitor.ConfigurePolicy(
		nil, []string{"spec.backendRefs.name=service"},
	); err != nil {
		t.Fatalf("configure graph reference policy: %v", err)
	}
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

	if monitor.canWatchVersion(context.Background(), gvr, "apps") {
		t.Fatal("forbidden custom resource should not start an informer")
	}
}

func TestMonitorStopClearsLifecycleState(t *testing.T) {
	canceled := false
	done := make(chan struct{})
	monitor := &Monitor{
		started:    true,
		generation: 4,
		cancel:     func() { canceled = true },
		done:       done,
		ctx:        context.Background(),
		factories: map[string]dynamicinformer.DynamicSharedInformerFactory{
			"widgets": nil,
		},
		stops: map[string]context.CancelFunc{
			"widgets": func() {},
		},
		crdVersions: map[string]map[string]struct{}{
			"widgets.example.io": {"widgets": {}},
		},
	}

	if err := monitor.Stop(context.Background()); err != nil {
		t.Fatalf("stop monitor: %v", err)
	}

	if !canceled {
		t.Fatal("Stop did not cancel the monitor context")
	}
	select {
	case <-done:
	default:
		t.Fatal("Stop did not signal monitor completion")
	}
	if monitor.started || monitor.cancel != nil || monitor.ctx != nil {
		t.Fatalf("Stop left lifecycle state active: %+v", monitor)
	}
	if len(monitor.factories) != 0 || len(monitor.stops) != 0 ||
		len(monitor.crdVersions) != 0 {
		t.Fatal("Stop left canceled informer state registered")
	}
}

func TestMonitorIgnoresStaleLifecycleReset(t *testing.T) {
	monitor := &Monitor{
		started:    true,
		generation: 2,
		ctx:        context.Background(),
		cancel:     func() {},
	}

	if err := monitor.resetLifecycle(context.Background(), 1); err != nil {
		t.Fatalf("reset lifecycle: %v", err)
	}

	if !monitor.started || monitor.generation != 2 || monitor.ctx == nil {
		t.Fatal("stale lifecycle reset changed the active generation")
	}
}

func TestStatusIncludesGenerationAndWatcherReason(t *testing.T) {
	monitor := &Monitor{
		started:       true,
		generation:    7,
		staticWatcher: nil,
	}
	status := monitor.Status()
	if status.Generation != 7 {
		t.Fatalf("status generation = %d, want 7", status.Generation)
	}
	if status.Reason != "source_not_configured" {
		t.Fatalf("status reason = %q, want source_not_configured", status.Reason)
	}
}

func TestLifecycleResetIsIdempotentWhenStopRacesCancellation(t *testing.T) {
	done := make(chan struct{})
	monitor := &Monitor{
		started:     true,
		generation:  3,
		done:        done,
		factories:   make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		stops:       make(map[string]context.CancelFunc),
		versionDone: make(map[string]chan struct{}),
		crdVersions: make(map[string]map[string]struct{}),
	}

	var group sync.WaitGroup
	group.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer group.Done()
			if err := monitor.resetLifecycle(context.Background(), 3); err != nil {
				t.Errorf("reset lifecycle: %v", err)
			}
		}()
	}
	group.Wait()

	if monitor.started {
		t.Fatal("racing lifecycle resets left the monitor started")
	}
	select {
	case <-done:
	default:
		t.Fatal("reset did not signal monitor completion")
	}
}
