package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// heldResolved is the resolve of a roll-up member whose page was held at
// notify because another member's page stood for the whole listing: no
// alert of its own was ever open at the pagers.
func heldResolved(name string) incident.Decision {
	d := resolved(name, true)
	d.Incident.Delivery.HoldAtNotify()
	return d
}

func ownMessages(seen []notification.Message) []notification.Message {
	var own []notification.Message
	for _, m := range seen {
		if m.Carrier == "" {
			own = append(own, m)
		}
	}
	return own
}

// Members of one roll-up whose pages were held beside the one page that
// stood for the listing resolve in one tick as one message: they have no
// thread and no alert of their own to close.
func TestEngineJointRecoveryGroupsHeldPageMembers(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	page := resolved("page", true)
	page.Incident.Tier = incident.Page
	page.Incident.Delivery.MarkPaged()

	e.announcer.announce(context.Background(), now, []incident.Decision{
		page, heldResolved("a"), heldResolved("b"), heldResolved("c")})

	own := ownMessages(*seen)
	if len(*seen) != 4 || len(own) != 2 {
		t.Fatalf("want the page and one joint resolve, got %d of %d: %+v",
			len(own), len(*seen), *seen)
	}
	joint := own[1]
	if !containsAll(joint.Note, "b in shop", "c in shop") {
		t.Fatalf("the joint resolve must name every member: %q", joint.Note)
	}
}

// A held-page member that spoke in a thread of its own, or whose alert is
// open at the pagers, resolves on its own.
func TestEngineJointRecoveryKeepsHeldPageThreadsAndAlerts(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	e.announcer.collect.Startup.Summary.Rollups = []announce.Listing{{
		Key: "rollup/x", Incidents: []string{"a", "spoke", "paged"},
		Followed: []string{"spoke"}}}
	paged := heldResolved("paged")
	paged.Incident.Delivery.MarkPaged()

	e.announcer.announce(context.Background(), now, []incident.Decision{
		heldResolved("a"), heldResolved("spoke"), paged})

	if len(*seen) != 3 || len(ownMessages(*seen)) != 3 {
		t.Fatalf("a thread or an open alert is never merged: %+v", *seen)
	}
}
