package pipeline

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestSyncWaitsReleasesAnEntityWhenItsKindSyncs(t *testing.T) {
	var w syncWaits
	hook := inventory.EntityID{Kind: "hook", Name: "h"}
	w.record(hook, []inventory.Kind{"service"})
	syncedKinds := map[inventory.Kind]bool{}
	synced := func(k inventory.Kind) bool { return syncedKinds[k] }

	if got := w.ready(synced); len(got) != 0 {
		t.Fatalf("released %v before the kind synced", got)
	}
	syncedKinds["service"] = true
	got := w.ready(synced)
	if len(got) != 1 || got[0] != hook {
		t.Fatalf("released %v, want the webhook", got)
	}
	if got := w.ready(synced); len(got) != 0 {
		t.Fatalf("released %v twice", got)
	}
}

func TestSyncWaitsForgetsAnEntityThatNoLongerWaits(t *testing.T) {
	var w syncWaits
	hook := inventory.EntityID{Kind: "hook", Name: "h"}
	w.record(hook, []inventory.Kind{"service"})
	w.record(hook, nil)

	if got := w.ready(nil); len(got) != 0 {
		t.Fatalf("released %v, want none", got)
	}
}

// The released entities are re-evaluated in this order: it must not depend
// on map iteration, or two runs of one cluster would differ.
func TestSyncWaitsReleasesEntitiesInOrderAndOnce(t *testing.T) {
	kinds := []inventory.Kind{"service", "secret", "configmap", "claim"}
	for round := 0; round < 30; round++ {
		var w syncWaits
		for _, name := range []string{"e", "b", "d", "a", "c"} {
			// Every entity waits for every kind.
			w.record(inventory.EntityID{Kind: "hook", Name: name}, kinds)
		}

		got := w.ready(nil)

		want := []string{"a", "b", "c", "d", "e"}
		if len(got) != len(want) {
			t.Fatalf("released %d entities, want %d once each: %v",
				len(got), len(want), got)
		}
		for i, id := range got {
			if id.Name != want[i] {
				t.Fatalf("round %d: released %v, want sorted %v",
					round, got, want)
			}
		}
	}
}
