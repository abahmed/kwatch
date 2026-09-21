package statuswatch

import (
	"context"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

func TestStaticProcessingReportsFailuresAndRecovery(t *testing.T) {
	sink := &statusSink{}
	monitor := &Monitor{
		incidentSink: sink, conditionRules: defaultConditionRules(),
		namespaceAllowed: func(namespace string) bool {
			return namespace != "blocked"
		},
		now: func() time.Time {
			return time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
		},
	}
	watched := staticWatch{
		resource: "flowschema",
		rules: map[string]map[string]bool{
			"Dangling": {"True": true},
		},
	}
	failing := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "flow", "namespace": "apps",
		},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Dangling", "status": "True"},
		}},
	}}
	monitor.processStatic(failing, watched)
	if len(sink.processed) != 1 {
		t.Fatalf("processed = %d, want 1", len(sink.processed))
	}
	healthy := failing.DeepCopy()
	healthy.Object["status"] = map[string]interface{}{}
	monitor.processStatic(healthy, watched)
	if len(sink.resolved) != 1 || sink.resolved[0].reason !=
		constant.ReasonAPIPriorityAndFairnessFailure {
		t.Fatalf("resolved = %#v", sink.resolved)
	}
	blocked := healthy.DeepCopy()
	blocked.SetNamespace("blocked")
	monitor.processStatic(blocked, watched)
	if len(sink.resolved) != 1 {
		t.Fatal("blocked resource changed sink")
	}
	monitor.processStatic("not an object", watched)
	monitor.resolveStatic(healthy, watched)
	if len(sink.resolved) != 2 {
		t.Fatalf("static delete resolution count = %d", len(sink.resolved))
	}
	monitor.resolveStatic("not an object", watched)
	apiService := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "v1.apps"},
	}}
	monitor.processAPIService(apiService)
	if len(sink.resolved) != 3 {
		t.Fatalf("healthy APIService resolution count = %d", len(sink.resolved))
	}
	request := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "request",
		},
		"status": map[string]interface{}{"conditions": []interface{}{
			map[string]interface{}{"type": "Approved", "status": "True"},
		}},
	}}
	request.SetCreationTimestamp(metav1.NewTime(
		time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC),
	))
	certificateWatch := staticWatch{resource: "certificatesigningrequest"}
	monitor.processStatic(request, certificateWatch)
	if len(sink.processed) != 2 {
		t.Fatalf("certificate processing count = %d", len(sink.processed))
	}
}

func TestCustomResourceAndCRDVersionLifecycleBranches(t *testing.T) {
	sink := &statusSink{}
	monitor := &Monitor{
		incidentSink: sink,
		namespaceAllowed: func(namespace string) bool {
			return namespace == "apps"
		},
		crdVersions: make(map[string]map[string]struct{}),
		factories:   make(map[string]dynamicwatch.Factory),
		stops:       make(map[string]context.CancelFunc),
		versionDone: make(map[string]chan struct{}),
	}
	monitor.processCR("not an object")
	monitor.resolveCR("not an object")
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "apps",
		},
	}}
	monitor.processCR(object)
	monitor.resolveCR(object)
	if len(sink.resolved) != 2 {
		t.Fatalf("custom resource resolutions = %d", len(sink.resolved))
	}
	monitor.resolveCR(&unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "blocked",
		},
	}})
	monitor.watchCRD("not an object")
	malformed := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "widgets.example.io"},
	}}
	monitor.watchCRD(malformed)
	if _, ok := monitor.crdVersions["widgets.example.io"]; !ok {
		t.Fatal("malformed CRD was not tracked")
	}
	valid := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "widgets.example.io"},
		"spec": map[string]interface{}{
			"group": "example.io", "scope": "Namespaced",
			"names": map[string]interface{}{"plural": "widgets"},
			"versions": []interface{}{map[string]interface{}{
				"name": "v1", "served": true,
				"subresources": map[string]interface{}{
					"status": map[string]interface{}{},
				},
			}},
		},
	}}
	monitor.watchAll = false
	monitor.namespaces = []string{"apps"}
	monitor.watchCRD(valid)
	if len(monitor.crdVersions["widgets.example.io"]) != 1 {
		t.Fatal("valid CRD version was not tracked")
	}
	monitor.deleteCRD(malformed)
	if _, ok := monitor.crdVersions["widgets.example.io"]; ok {
		t.Fatal("deleted CRD remained tracked")
	}
	monitor.deleteCRD("not an object")
	monitor.reconcileCRDVersions("widgets", map[string]struct{}{})
}

func TestStatusLifecycleStateHelpers(t *testing.T) {
	var monitor *Monitor
	if monitor.Status().State != "unavailable" {
		t.Fatal("nil monitor was not unavailable")
	}
	monitor = &Monitor{generation: 2}
	if _, _, ok := monitor.lifecycleContext(); ok ||
		monitor.currentContext() != nil {
		t.Fatal("stopped monitor had a lifecycle context")
	}
	if monitor.Status().State != "stopped" {
		t.Fatal("stopped monitor had unexpected status")
	}
	monitor.started = true
	monitor.ctx = context.Background()
	if ctx, generation, ok := monitor.lifecycleContext(); !ok ||
		ctx == nil || generation != 2 {
		t.Fatal("started monitor context was unavailable")
	}
	monitor.staticWatcher = nil
	if monitor.Status().State != "degraded" {
		t.Fatal("unconfigured started monitor was not degraded")
	}
	if got := versionKey(schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}, "apps"); got == "" {
		t.Fatal("version key was empty")
	}
	if _, _, err := cache.SplitMetaNamespaceKey("apps/widget"); err != nil {
		t.Fatal(err)
	}
}

func TestStatusLifecycleResetAndStartGuards(t *testing.T) {
	monitor := &Monitor{started: true, generation: 4}
	if err := monitor.Start(context.Background()); err != nil {
		t.Fatalf("already started Start() error = %v", err)
	}
	monitor.started = false
	monitor.resetting = true
	if err := monitor.Start(context.Background()); err == nil {
		t.Fatal("resetting Start() succeeded")
	}
	monitor = &Monitor{}
	if err := monitor.Stop(context.Background()); err != nil {
		t.Fatalf("stopped Stop() error = %v", err)
	}
	done := make(chan struct{})
	monitor = &Monitor{
		started: true, generation: 1, done: done,
		runWG:       &sync.WaitGroup{},
		factories:   make(map[string]dynamicwatch.Factory),
		stops:       make(map[string]context.CancelFunc),
		versionDone: make(map[string]chan struct{}),
		crdVersions: make(map[string]map[string]struct{}),
	}
	monitor.factories["resource"] = nil
	if err := monitor.resetLifecycle(context.Background(), 1); err != nil {
		t.Fatalf("resetLifecycle() error = %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("resetLifecycle did not close done")
	}
	if monitor.started {
		t.Fatal("resetLifecycle left monitor started")
	}
}

func TestStatusWatchesListableCRDVersionUntilStopped(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "WidgetList",
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitor := &Monitor{
		client: client, ctx: ctx, started: true, generation: 1,
		runWG:       &sync.WaitGroup{},
		factories:   make(map[string]dynamicwatch.Factory),
		stops:       make(map[string]context.CancelFunc),
		versionDone: make(map[string]chan struct{}),
	}
	monitor.watchVersion(gvr, "apps")
	key := versionKey(gvr, "apps")
	if monitor.factories[key] == nil {
		t.Fatal("listable CRD version was not started")
	}
	monitor.stopVersion(key)
	if monitor.factories[key] != nil {
		t.Fatal("stopped CRD version remained registered")
	}
}
