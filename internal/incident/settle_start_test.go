package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// A page for a failure that began before kwatch started waits the full
// settle: it may be clearing, and the short page settle is for news.
func TestPageFoundAtStartSettlesFully(t *testing.T) {
	r := newRig(t, Config{})
	f := sig(entity("node", "n1"), "NodeNotReady", detection.Critical)
	f.Since = at(0).Add(-10 * time.Minute)
	r.raise(at(0), f)

	wantNone(t, r.tick(at(DefaultPageSettle)))
	wantNone(t, r.tick(at(DefaultSettle-time.Second)))
	ds := r.tick(at(DefaultSettle))
	wantAction(t, ds, Announce, "settled")
	if ds[0].Incident.Tier != Page {
		t.Fatalf("tier = %v, want Page", ds[0].Incident.Tier)
	}
}
