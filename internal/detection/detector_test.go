package detection

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// unsyncedDetector asks about a kind that is not synced.
type unsyncedDetector struct{}

func (unsyncedDetector) Name() string { return "unsynced" }
func (unsyncedDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{AnyKind}
}
func (unsyncedDetector) Detect(
	ctx Context, _ inventory.Entity,
) []Finding {
	ctx.Synced("service")
	ctx.Synced("service")
	return nil
}

func TestEvaluationNamesTheUnsyncedKindsADetectorNeeded(t *testing.T) {
	r := NewRegistry(func(inventory.Kind) bool { return false },
		unsyncedDetector{})
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "hook", Name: "h"}

	got := r.Evaluate(model, time.Now(), id)

	if len(got.Unsynced) != 1 || got.Unsynced[0] != "service" {
		t.Fatalf("Unsynced = %v, want [service] once", got.Unsynced)
	}
}

// otherUnsynced asks about the same unsynced kind as unsyncedDetector.
type otherUnsynced struct{ unsyncedDetector }

func (otherUnsynced) Name() string { return "other-unsynced" }

// steadyDetector needs no other kind.
type steadyDetector struct{}

func (steadyDetector) Name() string { return "steady" }
func (steadyDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{AnyKind}
}
func (steadyDetector) Detect(Context, inventory.Entity) []Finding {
	return nil
}

// Every detector that needed an unsynced kind is named, even when an
// earlier detector already asked about the same kind, and only those.
func TestEvaluationNamesTheDetectorsThatHitAnUnsyncedKind(t *testing.T) {
	r := NewRegistry(func(inventory.Kind) bool { return false },
		unsyncedDetector{}, steadyDetector{}, otherUnsynced{})
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: "hook", Name: "h"}

	got := r.Evaluate(model, time.Now(), id)

	if len(got.UnsyncedBy) != 2 || got.UnsyncedBy[0] != "unsynced" ||
		got.UnsyncedBy[1] != "other-unsynced" {
		t.Fatalf("UnsyncedBy = %v, want [unsynced other-unsynced]",
			got.UnsyncedBy)
	}
}
