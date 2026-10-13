package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// resolved is the resolve of a workload incident that a roll-up named.
func resolved(name string, rolledUp bool) incident.Decision {
	root := inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "shop", Name: name}
	p := incident.Incident{ID: name, Root: root, Tier: incident.Notify,
		State: incident.Resolved}
	if rolledUp {
		p.Delivery.MarkRolledUp(1)
	}
	return incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: "healthy for 3m0s"}
}

// Resolves of incidents that only a roll-up named, in one tick, are one
// message: the first tells people about all of them, and the others are
// recorded as carried by it.
func TestEngineJointRecoverySendsOneMessage(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)

	e.announcer.announce(context.Background(), now, []incident.Decision{
		resolved("a", true), resolved("b", true), resolved("c", true)})

	var own []notification.Message
	for _, m := range *seen {
		if m.Carrier == "" {
			own = append(own, m)
		}
	}
	if len(*seen) != 3 || len(own) != 1 {
		t.Fatalf("want 1 message and 2 carried, got %d of %d: %+v",
			len(own), len(*seen), *seen)
	}
	if !own[0].Resolved() ||
		!containsAll(own[0].Note, "b in shop", "c in shop") {
		t.Fatalf("the message must name every recovery: %q", own[0].Note)
	}
	for _, m := range *seen {
		if m.Carrier != "" && m.Carrier != jointRecovery {
			t.Fatalf("carrier = %q", m.Carrier)
		}
	}
}

// Anything that is not a plain roll-up member resolve stays on its own: a
// lone one, one with its own thread, and a page.
func TestEngineJointRecoveryLeavesOtherResolvesAlone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	spoke := resolved("spoke", true)
	spoke.Incident.Delivery.MarkPaged()
	page := resolved("page", true)
	page.Incident.Tier = incident.Page

	e.announcer.announce(context.Background(), now, []incident.Decision{
		resolved("a", true), spoke, page, resolved("own", false)})

	if len(*seen) != 4 {
		t.Fatalf("want four separate messages, got %d", len(*seen))
	}
	for _, m := range *seen {
		if m.Carrier != "" {
			t.Fatalf("nothing is carried: %+v", m)
		}
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
