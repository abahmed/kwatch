package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// entityCounter counts the full entity snapshots a reader hands out.
type entityCounter struct {
	inventory.Reader
	snapshots int
}

func (c *entityCounter) Entity(id inventory.EntityID) (
	inventory.Entity, bool,
) {
	c.snapshots++
	return c.Reader.Entity(id)
}

func TestFingerprintsReadAttributesWithoutEntitySnapshots(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	model := modelWithDeployment(t, 3)
	counter := &entityCounter{Reader: model}

	got := Fingerprints(counter, now)

	if counter.snapshots != 0 {
		t.Errorf("Fingerprints copied %d entities, want none",
			counter.snapshots)
	}
	found := false
	for _, v := range got {
		if text, ok := v.(string); ok && strings.Contains(text, "replicas=3") {
			found = true
		}
	}
	if !found {
		t.Errorf("the deployment's fingerprint is missing: %v", got)
	}
}
