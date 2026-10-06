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
