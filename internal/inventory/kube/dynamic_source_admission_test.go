package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestObjectAdmissionPerKindCap(t *testing.T) {
	budget := &objectBudget{limit: 10}
	a := newAdmission(widgetGVR, 1, budget)

	assert.True(t, a.admit(widget("a")))
	assert.True(t, a.admit(widget("a")), "admitted objects stay admitted")
	assert.False(t, a.admit(widget("b")))
	assert.Equal(t, int64(1), a.refused.Load())

	// Recovery: deleting the admitted object frees room for another.
	assert.True(t, a.forget(widget("a")))
	assert.False(t, a.forget(widget("a")))
	assert.True(t, a.admit(widget("b")))
	assert.Equal(t, int64(1), budget.used.Load())

	a.releaseAll()
	assert.Zero(t, budget.used.Load())
	assert.False(t, a.admitted(widget("b")))
}

func TestObjectAdmissionSharesGlobalBudget(t *testing.T) {
	budget := &objectBudget{limit: 2}
	first := newAdmission(widgetGVR, 10, budget)
	second := newAdmission(crdResource, 10, budget)

	assert.True(t, first.admit(widget("a")))
	assert.True(t, second.admit(widget("b")))
	assert.False(t, first.admit(widget("c")))
	assert.Equal(t, int64(2), budget.used.Load())
}

func TestObjectAdmissionTombstones(t *testing.T) {
	a := newAdmission(widgetGVR, 10, &objectBudget{limit: 10})
	assert.True(t, a.admit(widget("a")))
	tombstone := cache.DeletedFinalStateUnknown{Key: "ns/a",
		Obj: widget("a")}
	assert.True(t, a.admitted(tombstone))
	assert.True(t, a.forget(tombstone))
	assert.False(t, a.admit(42), "objects without a key are refused")
}

func handlerFixture(perKind int) (
	cache.ResourceEventHandlerDetailedFuncs, *[]inventory.Observation,
) {
	var got []inventory.Observation
	w := &dynamicWatch{
		translator: NewTranslator(withGenericAttributes(
			NewUnstructuredSchema("example.com", "Widget"), WatchStatus)),
		admission: newAdmission(widgetGVR, perKind,
			&objectBudget{limit: 100}),
	}
	h := cappedHandler(context.Background(), w,
		func(_ context.Context, obs ...inventory.Observation) {
			got = append(got, obs...)
		},
		func() time.Time { return deletedAt })
	return h.(cache.ResourceEventHandlerDetailedFuncs), &got
}

func entities(obs []inventory.Observation) map[string]bool {
	out := map[string]bool{}
	for _, o := range obs {
		out[o.Entity.Name] = true
	}
	return out
}

func TestCappedHandlerObservesAdmittedObjectsOnly(t *testing.T) {
	h, got := handlerFixture(1)

	h.AddFunc(widget("a"), true)
	h.AddFunc(widget("b"), true)
	h.UpdateFunc(widget("b"), widget("b"))
	h.DeleteFunc(widget("b"))

	assert.Equal(t, map[string]bool{"a": true}, entities(*got))

	*got = nil
	h.DeleteFunc(widget("a"))
	h.UpdateFunc(widget("b"), widget("b"))
	assert.Equal(t, map[string]bool{"a": true, "b": true}, entities(*got))
}

func TestCappedHandlerDropsVersionOnlyUpdates(t *testing.T) {
	h, got := handlerFixture(10)
	old, renewed := widget("a"), widget("a")
	old.SetResourceVersion("1")
	renewed.SetResourceVersion("2")
	renewed.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "x"}})

	h.UpdateFunc(old, renewed)
	assert.Empty(t, *got)

	// A resync repeats the same version and is still observed.
	h.UpdateFunc(old, old)
	assert.NotEmpty(t, *got)
}

func TestVersionOnlyChange(t *testing.T) {
	meta := func(
		rv string, labels map[string]string,
	) *metav1.PartialObjectMetadata {
		return &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Name: "l", ResourceVersion: rv, Labels: labels,
		}}
	}
	withStatus := func(rv, phase string) *unstructured.Unstructured {
		u := widget("w")
		u.SetResourceVersion(rv)
		u.Object["status"] = map[string]any{"phase": phase}
		return u
	}
	tt := []struct {
		name     string
		old, new any
		want     bool
	}{
		{"metadata_renewal", meta("1", nil), meta("2", nil), true},
		{"metadata_label_change", meta("1", nil),
			meta("2", map[string]string{"a": "b"}), false},
		{"resync", meta("1", nil), meta("1", nil), false},
		{"status_unchanged", withStatus("1", "Ready"),
			withStatus("2", "Ready"), true},
		{"status_changed", withStatus("1", "Ready"),
			withStatus("2", "Failed"), false},
		{"mixed_types", meta("1", nil), withStatus("2", "Ready"), false},
		{"not_objects", "a", "b", false},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, versionOnlyChange(tc.old, tc.new))
		})
	}
}

func TestRediscoveryHandlerTriggers(t *testing.T) {
	requests := 0
	handler := rediscoveryHandler(func() { requests++ })
	h := handler.(cache.ResourceEventHandlerDetailedFuncs)
	crd := func(
		generation int64, established string,
	) *unstructured.Unstructured {
		u := widgetCRD()
		u.SetGeneration(generation)
		u.Object["status"] = map[string]any{"conditions": []any{
			map[string]any{"type": "Established", "status": established},
		}}
		return u
	}

	h.AddFunc(crd(1, "True"), true)
	assert.Zero(t, requests, "initial list does not rediscover")
	h.UpdateFunc(crd(1, "True"), crd(1, "True"))
	assert.Zero(t, requests, "status churn does not rediscover")

	h.AddFunc(crd(1, "False"), false)
	h.UpdateFunc(crd(1, "False"), crd(1, "True"))
	h.UpdateFunc(crd(1, "True"), crd(2, "True"))
	h.DeleteFunc(crd(2, "True"))
	assert.Equal(t, 4, requests)

	h.UpdateFunc("x", "y")
	assert.Equal(t, 4, requests)
}

func TestRequestDiscoveryCoalesces(t *testing.T) {
	src := NewDynamicSource(DynamicConfig{})
	src.requestDiscovery()
	src.requestDiscovery()
	assert.Len(t, src.trigger, 1)
}
