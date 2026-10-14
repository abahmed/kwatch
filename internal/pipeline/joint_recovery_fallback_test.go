package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// panicOnce is a history that panics the first time it is asked about
// the entity named name, which makes composing that decision fail once.
type panicOnce struct {
	inventory.HistoryReader
	name  string
	fired bool
}

func (p *panicOnce) Entity(id inventory.EntityID) (inventory.Entity, bool) {
	if id.Name == p.name && !p.fired {
		p.fired = true
		panic("compose failed")
	}
	return p.HistoryReader.Entity(id)
}

// When the joint message cannot be composed, no follower is lost: each
// one is sent as a resolve of its own, and nothing is marked as carried.
func TestEngineJointRecoveryFallsBackToOwnResolves(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	e.announcer.history = &panicOnce{
		HistoryReader: e.announcer.history, name: "b"}

	e.announcer.announce(context.Background(), now, []incident.Decision{
		resolved("a", true), resolved("b", true), resolved("c", true)})

	titles := map[string]bool{}
	for _, m := range *seen {
		if m.Carrier != "" || !m.Resolved() {
			t.Fatalf("every resolve goes on its own: %+v", m)
		}
		titles[m.Title] = true
	}
	if len(*seen) != 3 || len(titles) != 3 {
		t.Fatalf("want three own resolves, got %d: %+v", len(*seen), *seen)
	}
	if e.announcer.joint != nil {
		t.Fatalf("joint must be empty, got %v", e.announcer.joint)
	}
}

var _ = notification.Message{}
