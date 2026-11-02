package kube

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// scaleDownAll sets n workloads to zero replicas, all at at.
func scaleDownAll(w wakeModel, n int, at time.Time) {
	for i := range n {
		w.scale("app"+strconv.Itoa(i), "2", "0", at)
	}
}

func deploy(name string) inventory.EntityID {
	return inventory.EntityID{Kind: KindDeployment, Namespace: "shop",
		Name: name}
}

func TestScaledDownInBatchNeedsScaleDownMin(t *testing.T) {
	w := newWakeModel(t)
	scaleDownAll(w, ScaleDownMin, wakeNow)
	assert.True(t, ScaledDownInBatch(w.m, deploy("app0"), wakeNow))

	few := newWakeModel(t)
	scaleDownAll(few, ScaleDownMin-1, wakeNow)
	assert.False(t, ScaledDownInBatch(few.m, deploy("app0"), wakeNow))
}

func TestScaledDownInBatchIgnoresAnIsolatedScaleDown(t *testing.T) {
	w := newWakeModel(t)
	w.scale("lonely", "3", "0", wakeNow)
	assert.False(t, ScaledDownInBatch(w.m, deploy("lonely"), wakeNow))
}

func TestScaledDownInBatchEmptyInputIsFalse(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	assert.False(t, ScaledDownInBatch(m, deploy("app0"), wakeNow))
}

func TestScaledDownInBatchZeroTimeIsFalse(t *testing.T) {
	w := newWakeModel(t)
	scaleDownAll(w, ScaleDownMin, wakeNow)
	assert.False(t, ScaledDownInBatch(w.m, deploy("app0"), time.Time{}))
}

func TestScaledDownInBatchCountsStatefulSets(t *testing.T) {
	w := newWakeModel(t)
	scaleDownAll(w, ScaleDownMin-2, wakeNow)
	for _, name := range []string{"db0", "db1"} {
		id := inventory.EntityID{Kind: KindStatefulSet,
			Namespace: "shop", Name: name}
		w.apply(inventory.Observation{Kind: inventory.Observed,
			At: wakeNow, Entity: id})
		w.apply(inventory.Observation{Kind: inventory.Changed,
			At: wakeNow, Entity: id, Change: inventory.Change{
				Entity: id, At: wakeNow,
				Fields: []inventory.FieldChange{{
					Path: "spec.replicas", Before: "1",
					After: "0"}}}})
	}
	assert.True(t, ScaledDownInBatch(w.m, deploy("app0"), wakeNow))
}

func TestScaledDownInBatchCountsTheWorkloadItself(t *testing.T) {
	// The workload is one of the batch even before its own change is
	// recorded: four others plus itself reach the minimum.
	w := newWakeModel(t)
	scaleDownAll(w, ScaleDownMin-1, wakeNow)
	w.apply(inventory.Observation{Kind: inventory.Observed, At: wakeNow,
		Entity: deploy("self")})
	assert.True(t, ScaledDownInBatch(w.m, deploy("self"), wakeNow))
	assert.False(t, ScaledDownInBatch(w.m, deploy("other"), wakeNow))
}

func TestScaledDownInBatchIgnoresScaleUps(t *testing.T) {
	w := newWakeModel(t)
	for i := range ScaleDownMin {
		w.scale("app"+strconv.Itoa(i), "0", "2", wakeNow)
	}
	assert.False(t, ScaledDownInBatch(w.m, deploy("app0"), wakeNow))
}

func TestScaledToZeroNearWindowBoundaries(t *testing.T) {
	w := newWakeModel(t)
	w.scale("app0", "2", "0", wakeNow)
	id := deploy("app0")
	assert.True(t, scaledToZeroNear(w.m, id, wakeNow))
	assert.True(t, scaledToZeroNear(w.m, id, wakeNow.Add(ScaleDownSpan)))
	// The change is older than at-ScaleDownSpan: out of the window.
	assert.False(t, scaledToZeroNear(w.m, id,
		wakeNow.Add(ScaleDownSpan+time.Second)))
	// The change is after at+ScaleDownSpan: out of the window.
	assert.False(t, scaledToZeroNear(w.m, id,
		wakeNow.Add(-ScaleDownSpan-time.Second)))
	assert.True(t, scaledToZeroNear(w.m, id,
		wakeNow.Add(-ScaleDownSpan)))
}

func TestScaledToZeroNearOnlyWantsAFallToZero(t *testing.T) {
	w := newWakeModel(t)
	w.scale("up", "0", "3", wakeNow)
	w.scale("already", "0", "0", wakeNow)
	w.scale("fewer", "5", "2", wakeNow)
	w.scale("junk", "5", "none", wakeNow)
	for _, name := range []string{"up", "already", "fewer", "junk"} {
		assert.False(t, scaledToZeroNear(w.m, deploy(name), wakeNow),
			name)
	}
	assert.False(t, scaledToZeroNear(w.m, deploy("unknown"), wakeNow))
}
