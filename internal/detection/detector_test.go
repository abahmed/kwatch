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
