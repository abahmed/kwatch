package detection

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Watching names the objects whose bad state a detector waits out, so
// a restored incident keeps its grace while its finding is on its way
// back. A tracked reading is no bad state.
func TestRegistryWatchingFollowsOngoingBadStates(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "p"}
	_, _ = model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: start, Entity: id})
	bad, got := true, time.Time{}
	var found []bool
	var tracked []time.Time
	r := NewRegistry(nil,
		&onsetDetector{bad: &bad, firstSeen: start, got: &got},
		trackDetector{&found, &tracked})

	r.Evaluate(model, start, id)
	if !r.Watching(id) {
		t.Fatal("a bad state waited out is watched")
	}
	bad = false
	r.Evaluate(model, start.Add(time.Minute), id)
	if r.Watching(id) {
		t.Fatal("a tracked reading alone is not a bad state")
	}
	other := inventory.EntityID{Kind: "Pod", Namespace: "ns", Name: "q"}
	if r.Watching(other) {
		t.Fatal("an object never evaluated is not watched")
	}
}
