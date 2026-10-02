package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

var widgetGVR = schema.GroupVersionResource{
	Group: "example.com", Version: "v1", Resource: "widgets",
}

func widgetCRD() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]any{"name": "widgets.example.com"},
		"spec": map[string]any{
			"group": "example.com",
			"names": map[string]any{"kind": "Widget", "plural": "widgets"},
			"versions": []any{map[string]any{
				"name": "v1", "served": true, "storage": true,
				"subresources": map[string]any{"status": map[string]any{}},
			}},
		},
	}}
}

func widget(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{"name": name, "namespace": "ns"},
	}}
}

// dynamicFixture is a dynamic source over a fake client with one Widget.
type dynamicFixture struct {
	client       *dynamicfake.FakeDynamicClient
	disco        *lockedDiscovery
	src          *DynamicSource
	observations chan inventory.Observation
}

func newDynamicFixture() *dynamicFixture {
	return newDynamicFixtureWith(nil, widgetCRD(), widget("w"))
}

// newDynamicFixtureWith builds the fixture over objects; tune, when set,
// adjusts the configuration first.
func newDynamicFixtureWith(
	tune func(*DynamicConfig), objects ...runtime.Object,
) *dynamicFixture {
	kinds := map[schema.GroupVersionResource]string{
		crdResource:        "CustomResourceDefinitionList",
		apiServiceResource: "APIServiceList",
		widgetGVR:          "WidgetList",
	}
	f := &dynamicFixture{
		client: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			runtime.NewScheme(), kinds, objects...),
		disco: newLockedDiscovery(
			append(anchorLists(), widgetList())...),
		observations: make(chan inventory.Observation, 64),
	}
	cfg := DynamicConfig{
		Client: f.client, Discovery: f.disco,
		Now: func() time.Time {
			return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
		},
		Submit: func(_ context.Context, observations ...inventory.Observation) {
			for _, observation := range observations {
				f.observations <- observation
			}
		},
	}
	if tune != nil {
		tune(&cfg)
	}
	f.src = NewDynamicSource(cfg)
	return f
}

func (f *dynamicFixture) waitSynced(t *testing.T, ctx context.Context) {
	t.Helper()
	f.src.mu.Lock()
	w := f.src.running[widgetGVR]
	f.src.mu.Unlock()
	require.NotNil(t, w, "widget informer not started")
	require.True(t, cache.WaitForCacheSync(ctx.Done(), w.hasSynced))
}

func (f *dynamicFixture) watching(gvr schema.GroupVersionResource) bool {
	f.src.mu.Lock()
	defer f.src.mu.Unlock()
	_, ok := f.src.running[gvr]
	return ok
}

func (f *dynamicFixture) stop(cancel context.CancelFunc) {
	cancel()
	f.src.stopAll()
	f.src.wg.Wait()
}

func (f *dynamicFixture) goneObservations() []inventory.EntityID {
	var gone []inventory.EntityID
	for {
		select {
		case observation := <-f.observations:
			if observation.Kind == inventory.Gone {
				gone = append(gone, observation.Entity)
			}
		default:
			return gone
		}
	}
}

func TestDynamicSourceRetiredResourceReportsEntitiesGone(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	require.NoError(t, f.client.Tracker().Delete(crdResource, "",
		"widgets.example.com"))
	f.disco.set(anchorLists()...)

	f.src.reconcile(ctx)

	assert.False(t, f.watching(widgetGVR))
	assert.Contains(t, f.goneObservations(),
		inventory.NewEntityID("example.com", "widget", "ns", "w"))
}

func TestDynamicSourceKeepsWatchesWhenDiscoveryFails(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	f.disco.set()
	f.disco.fail(errors.New("apiserver unavailable"))

	f.src.reconcile(ctx)

	assert.True(t, f.watching(widgetGVR), "transient error dropped watch")
	assert.Empty(t, f.goneObservations())
	assert.False(t, f.src.Status().Complete)

	// Recovery: once discovery works again, a removed type is retired.
	f.disco.fail(nil)
	f.disco.set(anchorLists()...)
	f.src.reconcile(ctx)
	assert.False(t, f.watching(widgetGVR))
	assert.True(t, f.src.Status().Complete)
}

// A partial discovery starts what it found but retires nothing.
func TestDynamicSourcePartialDiscoveryRetiresNothing(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	f.disco.set(anchorLists()...)
	f.disco.fail(&discovery.ErrGroupDiscoveryFailed{
		Groups: map[schema.GroupVersion]error{
			{Group: "example.com", Version: "v1"}: errors.New("503"),
		},
	})

	f.src.reconcile(ctx)

	assert.True(t, f.watching(widgetGVR))
	assert.True(t, f.watching(crdResource))
}

func TestDynamicSourceShutdownDoesNotReportEntitiesGone(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	f.stop(cancel)

	assert.Empty(t, f.goneObservations())
}

// A retire that races shutdown returns without submitting.
func TestDynamicSourceRetireStopsWithContext(t *testing.T) {
	f := newDynamicFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &dynamicWatch{done: make(chan struct{})}

	f.src.retire(ctx, w)

	assert.Empty(t, f.goneObservations())
}
