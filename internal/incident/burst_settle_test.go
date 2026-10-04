package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// Three page-tier incidents opening together are a burst: none is
// announced at the page settle, so the cause they share can surface; all
// are announced at the full settle when none did.
func TestManagerBurstOfPagesTakesTheFullSettle(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0),
		sig(entity("node", "n1"), "NodeNotReady", detection.Critical),
		sig(entity("node", "n2"), "NodeNotReady", detection.Critical),
		sig(entity("node", "n3"), "NodeNotReady", detection.Critical))

	wantNone(t, r.tick(at(DefaultPageSettle)))
	wantNone(t, r.tick(at(DefaultSettle-time.Second)))
	ds := r.tick(at(DefaultSettle))

	if len(ds) != 3 {
		t.Fatalf("want three announcements at the full settle, got %+v", ds)
	}
	for _, d := range ds {
		if d.Action != Announce || d.Incident.Tier != Page {
			t.Fatalf("want a page announcement, got %+v", d)
		}
	}
}

// Two incidents are not a burst: a page still settles fast.
func TestManagerTwoPagesStillSettleFast(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0),
		sig(entity("node", "n1"), "NodeNotReady", detection.Critical),
		sig(entity("node", "n2"), "NodeNotReady", detection.Critical))

	ds := r.tick(at(DefaultPageSettle))

	if len(ds) != 2 {
		t.Fatalf("want two announcements at the page settle, got %+v", ds)
	}
}
