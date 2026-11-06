package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Only an open incident a listing carried is introduced, as an update
// for that reason; a resolved one or one that spoke on its own path
// (never carried) is not.
func TestManagerIntroducesOnlyListedOpenIncidents(t *testing.T) {
	r := newRig(t, Config{})
	root := inventory.CoreID(kube.KindPod, "shop", "web")
	r.m.Restore([]Record{{ID: "a", Tier: Notify, State: Open, Root: root,
		Opened: t0, Announced: t0, PagedKnown: true}}, time.Time{})

	if _, ok := r.m.Introduce("a"); ok {
		t.Fatal("introduced an incident no listing carried")
	}
	r.m.RecordRolledUp("a")
	d, ok := r.m.Introduce("a")
	if !ok || d.Action != Update || d.Reason != ReasonIntroduced {
		t.Fatalf("decision = %+v %v, want an introduction", d, ok)
	}
	if _, ok := r.m.Introduce("missing"); ok {
		t.Fatal("introduced an unknown incident")
	}
}

// A listed incident awaits its listing until the roll-up mark is set;
// a resolved or quiet one awaits nothing.
func TestAwaitsListing(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	id := r.only().ID
	if !r.m.AwaitsListing(id) {
		t.Fatal("an open incident not rolled up awaits its listing")
	}
	r.m.RecordRolledUp(id)
	if r.m.AwaitsListing(id) {
		t.Fatal("a rolled-up incident awaits nothing")
	}
	if r.m.AwaitsListing("gone") {
		t.Fatal("an unknown incident awaits nothing")
	}
}
