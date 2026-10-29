package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// A kind the source reports synced in the middle of a step may still
// have observations waiting in the queue: the gate says synced only
// after the next drain begins, and unsynced at once when watching stops.
func TestSyncGateConfirmsAKindOnlyAtTheNextDrain(t *testing.T) {
	live := map[inventory.Kind]bool{}
	gate := NewSyncGate(func(k inventory.Kind) bool { return live[k] })

	gate.Advance()
	live[kube.KindService] = true
	if gate.Synced(kube.KindService) {
		t.Fatal("a kind synced after the drain began counts as synced")
	}
	gate.Advance()
	if !gate.Synced(kube.KindService) {
		t.Fatal("a kind synced when the drain began is not synced")
	}
	live[kube.KindService] = false
	if gate.Synced(kube.KindService) {
		t.Fatal("a kind no longer watched still counts as synced")
	}
}

// syncedFinder raises its finding while failing is set, and says
// nothing at all while the Service kind is not synced.
type syncedFinder struct{ failing bool }

func (*syncedFinder) Name() string { return "synced-finder" }

func (*syncedFinder) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

func (d *syncedFinder) Detect(
	ctx detection.Context, _ inventory.Entity,
) []detection.Finding {
	if !ctx.Synced(kube.KindService) || !d.failing {
		return nil
	}
	return []detection.Finding{{Reason: "BackendMissing",
		Severity: detection.Warning}}
}

// A detector that cannot run because a kind it needs is not synced says
// nothing about its finding: the finding stays until the detector can
// look again, and clears only when it then finds the entity healthy.
func TestEngineKeepsAFindingWhileItsDetectorWaitsForSync(t *testing.T) {
	synced := true
	finder := &syncedFinder{failing: true}
	e := newTestEngine(t, &fakeClock{now: time.Now()}, func(
		context.Context, incident.Decision, notification.Message,
	) {
	}, func(deps *Dependencies) {
		deps.Detectors = detection.NewRegistry(
			func(inventory.Kind) bool { return synced }, finder)
	})
	id := inventory.CoreID(kube.KindPod, "shop", "web-0")
	now := time.Now()
	checks := newRechecks()
	dirty := e.apply([]inventory.Observation{seen(id, now, nil)})
	pass := func(at time.Duration) []string {
		e.evaluate(context.Background(), now.Add(at), dirty, checks)
		return activeReasons(e, id)
	}

	if got := pass(0); len(got) != 1 {
		t.Fatalf("first pass active = %v, want the finding", got)
	}
	synced = false
	if got := pass(time.Second); len(got) != 1 {
		t.Fatalf("unsynced pass active = %v: the finding was cleared "+
			"by a detector that could not look", got)
	}
	synced, finder.failing = true, false
	if got := pass(2 * time.Second); len(got) != 0 {
		t.Fatalf("healthy pass active = %v, want the finding cleared",
			got)
	}
}
