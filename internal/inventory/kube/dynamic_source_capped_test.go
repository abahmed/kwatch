package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestDynamicSourceCachesObjectsOverCapAsStubs(t *testing.T) {
	f := newDynamicFixtureWith(
		func(cfg *DynamicConfig) { cfg.MaxObjectsPerKind = 1 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	f.src.mu.Lock()
	store := f.src.running[widgetGVR].store
	f.src.mu.Unlock()
	full, stubs := 0, 0
	for _, obj := range store.List() {
		switch obj.(type) {
		case *unstructured.Unstructured:
			full++
		case *cappedStub:
			stubs++
		}
	}
	assert.Equal(t, 1, full, "one object within the cap is cached")
	assert.Equal(t, 1, stubs, "the object over the cap is a stub")

	status := f.src.Status()
	assert.Equal(t, 1, status.Capped)
	assert.Equal(t, 1, status.Reasons[ReasonObjectCap])
	assert.Contains(t, status.Kinds, DynamicKind{
		Resource: "widgets.example.com", Reason: ReasonObjectCap,
	})
}

func TestCapTransformAdmitsOnceRoomFrees(t *testing.T) {
	var got []inventory.Observation
	a := newAdmission(widgetGVR, 1, &objectBudget{limit: 100})
	w := &dynamicWatch{
		translator: NewTranslator(withGenericAttributes(
			NewUnstructuredSchema("example.com", "Widget"), WatchStatus)),
		admission: a,
	}
	h := cappedHandler(context.Background(), w,
		func(_ context.Context, obs ...inventory.Observation) {
			got = append(got, obs...)
		}, func() time.Time { return deletedAt },
	).(cache.ResourceEventHandlerDetailedFuncs)
	transform := capTransform(a, statusTransform)

	first, err := transform(widget("a"))
	require.NoError(t, err)
	stub, err := transform(widget("b"))
	require.NoError(t, err)
	require.IsType(t, &unstructured.Unstructured{}, first)
	require.True(t, isCappedStub(stub))
	require.True(t, a.capped())

	h.AddFunc(first, true)
	h.AddFunc(stub, true)
	assert.Equal(t, map[string]bool{"a": true}, entities(got))

	// Deleting the admitted object frees room for the stubbed one.
	got = nil
	h.DeleteFunc(first)
	readmitted, err := transform(widget("b"))
	require.NoError(t, err)
	require.False(t, isCappedStub(readmitted))
	h.UpdateFunc(stub, readmitted)

	assert.True(t, entities(got)["b"], "b observed once admitted")
	for _, o := range got {
		assert.NotEqual(t, inventory.Changed, o.Kind,
			"admission after a stub is not a change")
	}
	assert.False(t, a.capped())
}

func TestCappedStubKeepsIdentity(t *testing.T) {
	obj := widget("a")
	obj.SetResourceVersion("7")
	stub := newCappedStub(obj)

	key, err := cache.MetaNamespaceKeyFunc(stub)
	require.NoError(t, err)
	assert.Equal(t, "ns/a", key)
	copied := stub.(*cappedStub).DeepCopyObject()
	assert.True(t, isCappedStub(copied))
	assert.Equal(t, "7", copied.(*cappedStub).ResourceVersion)
	assert.Equal(t, 42, newCappedStub(42), "non-objects pass through")
}
