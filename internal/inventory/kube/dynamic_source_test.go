package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

const testWait = 10 * time.Second

// runningSource runs a dynamic source and records its reconcile passes.
type runningSource struct {
	src          *DynamicSource
	observations chan inventory.Observation
	passes       chan DynamicStatus
	seen         []inventory.Observation
	cancel       context.CancelFunc
	done         chan struct{}
}

func runDynamicSource(t *testing.T, cfg DynamicConfig) *runningSource {
	t.Helper()
	r := &runningSource{
		observations: make(chan inventory.Observation, 256),
		passes:       make(chan DynamicStatus, 16),
		done:         make(chan struct{}),
	}
	cfg.Now = func() time.Time {
		return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	}
	cfg.Submit = func(_ context.Context, obs ...inventory.Observation) {
		for _, o := range obs {
			r.observations <- o
		}
	}
	cfg.Reconciled = func(s DynamicStatus) { r.passes <- s }
	r.src = NewDynamicSource(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { r.src.Run(ctx); close(r.done) }()
	t.Cleanup(r.stop)
	return r
}

func (r *runningSource) stop() {
	r.cancel()
	<-r.done
}

// pass waits for the next discovery pass.
func (r *runningSource) pass(t *testing.T) DynamicStatus {
	t.Helper()
	select {
	case s := <-r.passes:
		return s
	case <-time.After(testWait):
		t.Fatal("no discovery pass")
		return DynamicStatus{}
	}
}

// observed waits for an observation of kind that matches. Observations
// read while waiting are kept, so waits can come in any order.
func (r *runningSource) observed(
	t *testing.T, kind inventory.ObservationKind,
	match func(inventory.Observation) bool,
) inventory.Observation {
	t.Helper()
	for _, o := range r.seen {
		if o.Kind == kind && match(o) {
			return o
		}
	}
	deadline := time.After(testWait)
	for {
		select {
		case o := <-r.observations:
			r.seen = append(r.seen, o)
			if o.Kind == kind && match(o) {
				return o
			}
		case <-deadline:
			t.Fatal("observation not seen")
			return inventory.Observation{}
		}
	}
}

func entityIs(id inventory.EntityID) func(inventory.Observation) bool {
	return func(o inventory.Observation) bool { return o.Entity == id }
}

func dynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), map[schema.GroupVersionResource]string{
			crdResource:        "CustomResourceDefinitionList",
			apiServiceResource: "APIServiceList",
			widgetGVR:          "WidgetList",
		}, objects...)
}

func TestDynamicSourceWatchesDiscoveredResources(t *testing.T) {
	apiService := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": "v1.metrics.example"},
		"spec": map[string]any{"service": map[string]any{
			"namespace": "sys", "name": "metrics",
		}},
	}}
	r := runDynamicSource(t, DynamicConfig{
		Client: dynamicClient(widgetCRD(), apiService, widget("w")),
		Discovery: newLockedDiscovery(
			append(anchorLists(), widgetList())...),
	})

	status := r.pass(t)
	assert.Equal(t, 3, status.Watched[WatchStatus])
	assert.True(t, status.Complete)
	r.observed(t, inventory.Observed,
		entityIs(inventory.NewEntityID("", "apiservice", "",
			"v1.metrics.example")))
	o := r.observed(t, inventory.Observed,
		entityIs(inventory.NewEntityID("example.com", "widget", "ns", "w")))
	assert.Equal(t, inventory.Text("status"), o.Attributes[AttrWatchMode])
}

func TestDynamicSourceRediscoversOnCRDChange(t *testing.T) {
	client := dynamicClient()
	crds := watch.NewFake()
	client.PrependWatchReactor("customresourcedefinitions",
		func(ktesting.Action) (bool, watch.Interface, error) {
			return true, crds, nil
		})
	disco := newLockedDiscovery(anchorLists()...)
	r := runDynamicSource(t, DynamicConfig{
		Client: client, Discovery: disco, Rediscovery: time.Hour,
	})
	assert.Equal(t, 2, r.pass(t).Watched[WatchStatus])

	// A new CRD: discovery serves it once it is established.
	require.NoError(t, client.Tracker().Add(widget("w")))
	disco.set(append(anchorLists(), widgetList())...)
	crds.Add(widgetCRD())
	assert.Equal(t, 3, r.pass(t).Watched[WatchStatus])
	widgetID := inventory.NewEntityID("example.com", "widget", "ns", "w")
	r.observed(t, inventory.Observed,
		entityIs(widgetID))

	// The CRD is removed: its type is retired and its entities gone.
	disco.set(anchorLists()...)
	crds.Delete(widgetCRD())
	assert.Equal(t, 2, r.pass(t).Watched[WatchStatus])
	r.observed(t, inventory.Gone,
		entityIs(widgetID))
}

func TestDynamicSourceBudgetSkipsLowPriorityTypes(t *testing.T) {
	r := runDynamicSource(t, DynamicConfig{
		Client: dynamicClient(widgetCRD()),
		Discovery: newLockedDiscovery(
			append(anchorLists(), widgetList())...),
		Budget: 2,
	})

	status := r.pass(t)

	assert.Equal(t, 1, status.Skipped)
	assert.Equal(t, 2, status.Watched[WatchStatus])
	assert.False(t, r.src.running[widgetGVR] != nil)
}

func TestDynamicSourceRefusedTypeIsUnavailable(t *testing.T) {
	client := dynamicClient(widgetCRD(), widget("w"))
	client.PrependReactor("list", "widgets",
		func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, forbidden("widgets")
		})
	r := runDynamicSource(t, DynamicConfig{
		Client: client,
		Discovery: newLockedDiscovery(
			append(anchorLists(), widgetList())...),
	})
	r.pass(t)

	require.Eventually(t, func() bool {
		return r.src.Status().Unavailable == 1
	}, testWait, 10*time.Millisecond)
	assert.Equal(t, 2, r.src.Status().Watched[WatchStatus])
}

func TestDynamicSourceMetadataWithoutClientIsUnavailable(t *testing.T) {
	r := runDynamicSource(t, DynamicConfig{
		Client: dynamicClient(),
		Discovery: newLockedDiscovery(apiList("coordination.k8s.io/v1",
			servedResource("leases", "Lease", false))),
	})

	status := r.pass(t)

	assert.Equal(t, 1, status.Unavailable)
	assert.Zero(t, status.Watched[WatchMetadata])
}

func TestDynamicSourceDefaults(t *testing.T) {
	src := NewDynamicSource(DynamicConfig{})
	assert.Equal(t, DefaultResourceBudget, src.cfg.Budget)
	assert.Equal(t, DefaultMaxObjectsPerKind, src.cfg.MaxObjectsPerKind)
	assert.Equal(t, int64(DefaultObjectBudget), src.budget.limit)
	assert.Equal(t, DefaultRediscoveryInterval, src.cfg.Rediscovery)
}
