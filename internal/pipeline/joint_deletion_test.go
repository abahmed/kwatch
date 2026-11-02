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

// deletedResolve is the resolve of a roll-up member whose root was
// deleted.
func deletedResolve(name string) incident.Decision {
	d := resolved(name, true)
	d.Incident.Removed = []inventory.EntityID{d.Incident.Root}
	return d
}

// own returns the messages delivered on their own, not carried.
func own(seen []notification.Message) []notification.Message {
	var out []notification.Message
	for _, m := range seen {
		if m.Carrier == "" {
			out = append(out, m)
		}
	}
	return out
}

// Deletions of roll-up members in one tick are one message under
// "Deleted at the same moment", never under "Recovered", and the
// recoveries of the same tick get a message of their own.
func TestEngineJointDeletionSendsOneMessagePerEnd(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	gone := resolved("gone", true)
	gone.Incident.NamespaceRemoved = true

	e.announcer.announce(context.Background(), now, []incident.Decision{
		resolved("a", true), deletedResolve("x"), resolved("b", true),
		deletedResolve("y"), gone})

	sent := own(*seen)
	if len(*seen) != 5 || len(sent) != 2 {
		t.Fatalf("want 2 messages and 3 carried, got %d of %d: %+v",
			len(sent), len(*seen), *seen)
	}
	var recovered, deleted string
	for _, m := range sent {
		if strings.Contains(m.Note, "Deleted at the same moment") {
			deleted = m.Note
		} else {
			recovered = m.Note
		}
	}
	if !containsAll(deleted, "x in shop", "y in shop was deleted",
		"gone in shop was deleted with its namespace") ||
		strings.Contains(deleted, "Recovered") {
		t.Fatalf("deletions: %q", deleted)
	}
	if !containsAll(recovered, "Recovered at the same moment",
		"b in shop") || strings.Contains(recovered, "deleted") {
		t.Fatalf("recoveries: %q", recovered)
	}
}

// A resolve that a listing named speaks of the listed subject: when
// that subject was deleted it joins the deletions even though its root
// was not deleted.
func TestEngineJointDeletionFollowsTheListedSubject(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	listed := resolved("b", true)
	subject := inventory.EntityID{Kind: kube.KindDeployment,
		Namespace: "shop", Name: "listed"}
	listed.Incident.Subject = subject
	listed.Incident.Removed = []inventory.EntityID{subject}
	e.announcer.collect.Startup.Summary.Incidents = []string{"b"}

	e.announcer.announce(context.Background(), now, []incident.Decision{
		deletedResolve("a"), listed})

	sent := own(*seen)
	if len(sent) != 1 || !containsAll(sent[0].Note,
		"Deleted at the same moment", "listed in shop was deleted") {
		t.Fatalf("want one deletion message: %+v", *seen)
	}
}

// A resolve where only something other than what it speaks of was
// deleted, and one at the still-broken cap, are neither recoveries nor
// deletions: each stays on its own.
func TestEngineJointDeletionLeavesOtherEndsAlone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	member := resolved("member", true)
	member.Incident.Removed = []inventory.EntityID{{
		Kind: kube.KindPod, Namespace: "shop", Name: "member-1"}}
	capped := deletedResolve("capped")
	capped.Reason = incident.Reason(incident.StoppedTrackingPrefix +
		"2h; coverage check continues")
	node := resolved("node", true)
	node.Incident.Root = inventory.EntityID{Kind: kube.KindNode, Name: "n1"}
	node.Incident.Removed = []inventory.EntityID{node.Incident.Root}

	e.announcer.announce(context.Background(), now, []incident.Decision{
		deletedResolve("a"), member, capped, node})

	if len(*seen) != 4 || len(own(*seen)) != 4 {
		t.Fatalf("want four separate messages, got %+v", *seen)
	}
	for _, m := range *seen {
		if strings.Contains(m.Note, "same moment") {
			t.Fatalf("nothing is grouped: %q", m.Note)
		}
	}
}
