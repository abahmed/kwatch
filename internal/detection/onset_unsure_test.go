package detection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// gatedDetector asks for its onset, unless gate says it cannot judge:
// then it returns early, as for an unsynced kind or a stale source.
type gatedDetector struct {
	name string
	gate func(ctx Context) bool
	bad  *bool
	got  *time.Time
}

func (d gatedDetector) Name() string          { return d.name }
func (gatedDetector) Kinds() []inventory.Kind { return []inventory.Kind{"Pod"} }
func (d gatedDetector) Detect(ctx Context, _ inventory.Entity) []Finding {
	if !d.gate(ctx) {
		return nil
	}
	if *d.bad {
		*d.got = ctx.Onset(d.name, ctx.Now)
	}
	return nil
}

func onsetFixture() (*inventory.Model, inventory.EntityID, time.Time) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: start, Entity: id})
	return model, id, start
}

func TestRegistryKeepsOnsetOfDetectorReturningEarlyUnsynced(t *testing.T) {
	model, id, start := onsetFixture()
	synced := true
	bad, got := true, time.Time{}
	d := gatedDetector{name: "gated", bad: &bad, got: &got,
		gate: func(ctx Context) bool { return ctx.Synced("Node") }}
	r := NewRegistry(func(inventory.Kind) bool { return synced }, d)

	r.Evaluate(model, start, id)
	synced = false
	ev := r.Evaluate(model, start.Add(time.Minute), id)
	assert.Equal(t, []string{"gated"}, ev.UnsyncedBy)
	synced = true
	r.Evaluate(model, start.Add(2*time.Minute), id)
	assert.Equal(t, start, got, "the unsynced pass must not restart it")
}

func TestRegistryKeepsOnsetOfUndecidedDetector(t *testing.T) {
	model, id, start := onsetFixture()
	stale := false
	bad, got := true, time.Time{}
	d := gatedDetector{name: "stale", bad: &bad, got: &got,
		gate: func(ctx Context) bool {
			if stale {
				ctx.Undecided()
			}
			return !stale
		}}
	r := NewRegistry(nil, d)

	r.Evaluate(model, start, id)
	stale = true
	ev := r.Evaluate(model, start.Add(time.Minute), id)
	assert.Equal(t, []string{"stale"}, ev.UnsyncedBy)
	assert.Empty(t, ev.Unsynced, "a stale source is no unsynced kind")
	stale = false
	r.Evaluate(model, start.Add(2*time.Minute), id)
	assert.Equal(t, start, got)
}

func TestRegistryForgetsOtherDetectorsOnsetsWhenOneIsUnsure(t *testing.T) {
	model, id, start := onsetFixture()
	synced := true
	gatedBad, otherBad := true, true
	var gatedGot, otherGot time.Time
	gated := gatedDetector{name: "gated", bad: &gatedBad, got: &gatedGot,
		gate: func(ctx Context) bool { return ctx.Synced("Node") }}
	other := gatedDetector{name: "other", bad: &otherBad, got: &otherGot,
		gate: func(Context) bool { return true }}
	r := NewRegistry(
		func(inventory.Kind) bool { return synced }, gated, other)

	r.Evaluate(model, start, id)
	synced, otherBad = false, false
	r.Evaluate(model, start.Add(time.Minute), id)
	otherBad = true
	later := start.Add(2 * time.Minute)
	r.Evaluate(model, later, id)
	assert.Equal(t, later, otherGot,
		"a cleared state of a detector that ran must start over")
}
