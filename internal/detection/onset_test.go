package detection

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// onsetDetector asks for one bad state while bad is true.
type onsetDetector struct {
	bad       *bool
	firstSeen time.Time
	got       *time.Time
}

func (onsetDetector) Name() string            { return "onset" }
func (onsetDetector) Kinds() []inventory.Kind { return []inventory.Kind{"Pod"} }
func (d onsetDetector) Detect(ctx Context, _ inventory.Entity) []Finding {
	if *d.bad {
		*d.got = ctx.Onset("bad", d.firstSeen)
	}
	return nil
}

func TestRegistryOnsetIsKeptWhileAskedAndForgottenAfter(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: start, Entity: id})
	bad, got := true, time.Time{}
	d := &onsetDetector{bad: &bad, firstSeen: start, got: &got}
	r := NewRegistry(nil, d)

	r.Evaluate(model, start, id)
	d.firstSeen = start.Add(time.Minute)
	r.Evaluate(model, start.Add(time.Minute), id)
	if !got.Equal(start) {
		t.Fatalf("onset = %v, want the first sighting %v", got, start)
	}
	bad = false
	r.Evaluate(model, start.Add(2*time.Minute), id)
	bad = true
	r.Evaluate(model, start.Add(3*time.Minute), id)
	if !got.Equal(d.firstSeen) {
		t.Fatalf("onset = %v, want a fresh start after recovery", got)
	}
	if len(r.onsets.byEntity) != 1 {
		t.Fatalf("onsets = %d entities, want 1", len(r.onsets.byEntity))
	}
	bad = false
	r.Evaluate(model, start.Add(4*time.Minute), id)
	if len(r.onsets.byEntity) != 0 {
		t.Fatal("a state nobody asks for must be forgotten")
	}
}

func TestContextOnsetWithoutRegistryReturnsFirstSeen(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := Context{Now: now}
	if got := ctx.Onset("bad", time.Time{}); !got.Equal(now) {
		t.Fatalf("zero first sighting = %v, want now", got)
	}
}
