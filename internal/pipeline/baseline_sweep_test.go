package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Warning counters of entities that were never observed (or whose Gone
// came first) are swept, so the map cannot grow with pod churn.
func TestBaselineSamplerSweepsWarningsOfGoneEntities(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	sampler := newBaselineSampler(model)
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	for i := range 3 * warningSweepFloor {
		id := inventory.EntityID{
			Kind: kube.KindPod, Namespace: "ns", Name: fmt.Sprintf("p%d", i),
		}
		sampler.observe(inventory.Observation{
			Kind: inventory.Noted, Entity: id, At: at,
			Note: inventory.Note{Warning: true, Reason: "BackOff", Count: 1},
		})
	}
	if got := len(sampler.warnings); got > 2*warningSweepFloor {
		t.Fatalf("%d counters kept for entities that are gone, want <= %d",
			got, 2*warningSweepFloor)
	}
}
