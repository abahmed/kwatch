package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

func digestAnnounce(id string) incident.Decision {
	d := announce(id)
	d.Incident.Tier = incident.Digest
	return d
}

// digestHarness is an announcer whose sink keeps every message.
func digestHarness(
	t *testing.T, now time.Time,
) (*Engine, *[]notification.Message) {
	t.Helper()
	var sent []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		if m.Carrier == "" {
			sent = append(sent, m)
		}
	}
	return newTestEngine(t, &fakeClock{now: now}, sink, nil), &sent
}

// Digest-tier announcements wait for the window, then go as one message
// that paging providers never receive.
func TestEngineDigestCollectsLowTierIntoOneMessage(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, sent := digestHarness(t, now)

	rest, flushed := e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), digestAnnounce("b"),
			announce("urgent")})

	if flushed || len(rest) != 1 || rest[0].Incident.ID != "urgent" {
		t.Fatalf("only the notify incident passes through, rest = %+v", rest)
	}
	if len(*sent) != 0 || e.announcer.nextDigest() != now.Add(digestWindow) {
		t.Fatalf("nothing is sent before the window; next = %v",
			e.announcer.nextDigest())
	}
	_, flushed = e.announcer.collectDigest(context.Background(),
		now.Add(digestWindow), nil)
	if !flushed || len(*sent) != 1 {
		t.Fatalf("want one digest at the window's end, got %d", len(*sent))
	}
	msg := (*sent)[0]
	if !msg.IsSummary() || !strings.HasPrefix(msg.Key, "digest/") ||
		msg.Status != notification.StatusLow {
		t.Fatalf("digest = %+v", msg)
	}
	if !strings.Contains(msg.Note, "two") {
		t.Fatalf("digest must count both problems: %s", msg.Note)
	}
	if !e.announcer.nextDigest().IsZero() {
		t.Fatal("a sent digest leaves nothing pending")
	}
}

// An update of a digest-tier incident folds into the pending entry; a
// resolve before the digest drops the entry; a resolve after it is
// listed in the next digest.
func TestEngineDigestFoldsUpdatesAndResolves(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, sent := digestHarness(t, now)
	update := digestAnnounce("a")
	update.Action, update.Incident.Revision = incident.Update, 2
	resolveB := digestAnnounce("b")
	resolveB.Action = incident.Resolve

	e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), digestAnnounce("b")})
	rest, _ := e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{update, resolveB})

	if len(rest) != 0 {
		t.Fatalf("digest decisions leaked: %+v", rest)
	}
	opened := e.announcer.digest.opened
	if len(opened) != 1 || opened[0].Incident.ID != "a" ||
		opened[0].Incident.Revision != 2 {
		t.Fatalf("pending = %+v, want a at revision 2 only", opened)
	}
	e.announcer.collectDigest(context.Background(), now.Add(digestWindow),
		nil)
	resolveA := digestAnnounce("a")
	resolveA.Action = incident.Resolve
	rest, _ = e.announcer.collectDigest(context.Background(),
		now.Add(digestWindow+time.Minute), []incident.Decision{resolveA})
	if len(rest) != 0 || len(e.announcer.digest.resolved) != 1 {
		t.Fatalf("a resolve after the digest waits for the next one: %+v",
			rest)
	}
	if len(*sent) != 1 {
		t.Fatalf("want exactly the first digest so far, got %d", len(*sent))
	}
}

// An incident promoted out of the digest tier before its digest is
// announced at once, even when the promotion arrives as an update.
func TestEngineDigestReleasesPromotedIncident(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, _ := digestHarness(t, now)
	e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a")})
	promoted := announce("a")
	promoted.Action, promoted.Incident.Revision = incident.Update, 2

	rest, _ := e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{promoted})

	if len(rest) != 1 || rest[0].Action != incident.Announce {
		t.Fatalf("promotion must announce, got %+v", rest)
	}
	if len(e.announcer.digest.opened) != 0 {
		t.Fatal("the promoted incident must leave the digest")
	}
}

// A held digest-tier decision still reaches the sink, marked as carried
// by the digest, so the audit log records it when it is made.
func TestEngineDigestRecordsHeldDecisionsForTheAuditLog(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var carried []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		if m.Carrier != "" {
			carried = append(carried, m)
		}
	}
	e := newTestEngine(t, &fakeClock{now: now}, sink, nil)

	e.announcer.collectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), announce("urgent")})

	if len(carried) != 1 || carried[0].Key != announce("a").Incident.ID ||
		carried[0].Carrier != "digest" {
		t.Fatalf("want the held announcement marked carried, got %+v",
			carried)
	}
}
