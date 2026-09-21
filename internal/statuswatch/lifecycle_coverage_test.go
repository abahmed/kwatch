package statuswatch

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

func TestStatusLifecycleHelpersCoverScopesAndContexts(t *testing.T) {
	monitor := &Monitor{namespaces: []string{"apps", "system"}}
	if got := monitor.watchNamespaces(false); len(got) != 1 || got[0] != "" {
		t.Fatalf("cluster scope = %v", got)
	}
	got := monitor.watchNamespaces(true)
	if len(got) != 2 || got[0] != "apps" {
		t.Fatalf("namespace scope = %v", got)
	}
	monitor.watchAll = true
	if got := monitor.watchNamespaces(true); len(got) != 1 || got[0] != "" {
		t.Fatalf("watch-all scope = %v", got)
	}
	key := versionKey(schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}, "apps")
	if key != "example.io/v1, Resource=widgets|apps" {
		t.Fatalf("version key = %q", key)
	}
	if ctx, cancel := lifecycleStopContext(context.Background()); ctx == nil {
		t.Fatal("provided stop context was lost")
	} else {
		cancel()
	}
	ctx, cancel := lifecycleStopContext(nil)
	cancel()
	if ctx == nil {
		t.Fatal("default stop context was nil")
	}

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor = NewWithClientsAndClock(
		dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), nil, nil,
		time.Minute, clock.Func(func() time.Time { return now }),
	)
	if !monitor.nowTime().Equal(now) {
		t.Fatalf("monitor clock = %v", monitor.nowTime())
	}
	if err := monitor.ConfigureSources(Sources{WatchAll: true}); err != nil {
		t.Fatalf("configure sources: %v", err)
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("sources were reconfigured")
	}
}

func TestStatusStaticWatcherSpecsAndVersionAvailability(t *testing.T) {
	monitor := &Monitor{}
	if err := monitor.startStaticWatcher(context.Background()); err == nil {
		t.Fatal("unconfigured static watcher did not fail")
	}
	monitor = &Monitor{watchAll: false, namespaces: []string{"apps"}}
	specs := monitor.staticWatchSpecs(context.Background())
	if len(specs) < 2 {
		t.Fatalf("static watcher specs = %d", len(specs))
	}
	if endpointSlicesGVR().Resource != "endpointslices" {
		t.Fatalf("endpoint slice GVR = %v", endpointSlicesGVR())
	}
	if monitor.admissionPolicySpec().GVR.Resource == "" ||
		monitor.admissionBindingSpec().GVR.Resource == "" {
		t.Fatal("admission watcher specs were incomplete")
	}

	gvr := schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			gvr: "WidgetList",
		},
	)
	monitor.client = client
	if !monitor.canWatchVersion(context.Background(), gvr, "apps") {
		t.Fatal("listable custom resource was rejected")
	}
	if monitor.canWatchVersion(nil, gvr, "apps") {
		t.Fatal("nil context was accepted")
	}
}

func TestStatusVersionCleanupIsSafeForMissingEntries(t *testing.T) {
	monitor := &Monitor{
		factories:   make(map[string]dynamicwatch.Factory),
		stops:       make(map[string]context.CancelFunc),
		versionDone: make(map[string]chan struct{}),
		crdVersions: make(map[string]map[string]struct{}),
	}
	monitor.stopVersion("missing")
	monitor.reconcileCRDVersions("widgets", map[string]struct{}{})
	if _, ok := monitor.crdVersions["widgets"]; !ok {
		t.Fatal("CRD version state was not recorded")
	}
}
