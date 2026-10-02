package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// An incident announced while out of scope was never heard. When it comes
// into scope, its first delivered message must introduce it.
func TestEngineUpdateEnteringScopeIsDeliveredAsAnnounce(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.InScope = func(incident.Incident) bool { return true }
	})
	hidden := incident.Incident{ID: "p1", Scope: incident.ScopeOut}
	heard := incident.Incident{ID: "p2", Scope: incident.ScopeIn}

	got := h.engine.announcer.inScope([]incident.Decision{
		{Action: incident.Update, Incident: hidden},
		{Action: incident.Update, Incident: heard},
	})

	if len(got) != 2 {
		t.Fatalf("want both decisions kept, got %d", len(got))
	}
	if got[0].Action != incident.Announce {
		t.Fatalf("hidden incident action = %v, want announce",
			got[0].Action)
	}
	if got[1].Action != incident.Update {
		t.Fatalf("heard incident action = %v, want update", got[1].Action)
	}
}
