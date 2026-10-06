package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

func digestAnnounce(id string) incident.Decision {
	d := announcement(id)
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

	rest, flushed := e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), digestAnnounce("b"),
			announcement("urgent")})

	if flushed || len(rest) != 1 || rest[0].Incident.ID != "urgent" {
		t.Fatalf("only the notify incident passes through, rest = %+v", rest)
	}
	due := now.Add(announce.DigestWindow)
	if len(*sent) != 0 || e.announcer.collect.NextDigest() != due {
		t.Fatalf("nothing is sent before the window; next = %v",
			e.announcer.collect.NextDigest())
	}
	_, flushed = e.announcer.collect.CollectDigest(context.Background(),
		now.Add(announce.DigestWindow), nil)
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
	if !e.announcer.collect.NextDigest().IsZero() {
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

	e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), digestAnnounce("b")})
	rest, _ := e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{update, resolveB})

	if len(rest) != 0 {
		t.Fatalf("digest decisions leaked: %+v", rest)
	}
	opened := e.announcer.collect.Low.Opened
	if len(opened) != 1 || opened[0].Incident.ID != "a" ||
		opened[0].Incident.Revision != 2 {
		t.Fatalf("pending = %+v, want a at revision 2 only", opened)
	}
	e.announcer.collect.CollectDigest(context.Background(),
		now.Add(announce.DigestWindow),
		nil)
	resolveA := digestAnnounce("a")
	resolveA.Action = incident.Resolve
	rest, _ = e.announcer.collect.CollectDigest(context.Background(),
		now.Add(announce.DigestWindow+time.Minute), []incident.Decision{resolveA})
	if len(rest) != 0 || len(e.announcer.collect.Low.Resolved) != 1 {
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
	e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a")})
	promoted := announcement("a")
	promoted.Action, promoted.Incident.Revision = incident.Update, 2

	rest, _ := e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{promoted})

	if len(rest) != 1 || rest[0].Action != incident.Announce {
		t.Fatalf("promotion must announce, got %+v", rest)
	}
	if len(e.announcer.collect.Low.Opened) != 0 {
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

	e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a"), announcement("urgent")})

	if len(carried) != 1 || carried[0].Key != announcement("a").Incident.ID ||
		carried[0].Carrier != "digest" {
		t.Fatalf("want the held announcement marked carried, got %+v",
			carried)
	}
}

// Configuration risks ride along a digest that goes out anyway, each
// named once; the next digest does not repeat them.
func TestEngineDigestNamesNewRisksOnce(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, sent := digestHarness(t, now)
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	e.announcer.collect.SetAdvisories(func() []detection.Finding {
		return []detection.Finding{{Entity: orders,
			Reason: reasons.RiskSingleReplica, Advisory: true,
			Summary: "It runs a single replica, so any restart is downtime"}}
	})

	e.announcer.collect.CollectDigest(context.Background(), now,
		[]incident.Decision{digestAnnounce("a")})
	e.announcer.collect.CollectDigest(context.Background(),
		now.Add(announce.DigestWindow),
		nil)
	e.announcer.collect.CollectDigest(context.Background(),
		now.Add(announce.DigestWindow+time.Minute),
		[]incident.Decision{digestAnnounce("b")})
	e.announcer.collect.CollectDigest(context.Background(),
		now.Add(2*announce.DigestWindow+time.Minute), nil)

	if len(*sent) != 2 {
		t.Fatalf("want two digests, got %d", len(*sent))
	}
	if !strings.Contains((*sent)[0].Note, "1 workload runs a single "+
		"replica (orders)") {
		t.Fatalf("the first digest must name the risk: %s", (*sent)[0].Note)
	}
	if strings.Contains((*sent)[1].Note, "Configuration risks") {
		t.Fatalf("a named risk must not repeat: %s", (*sent)[1].Note)
	}
}

// Risks of system namespaces and of kwatch's own namespace are not the
// team's to fix: the digest leaves them out.
func TestEngineDigestSkipsSystemAndOwnNamespaceRisks(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	t.Setenv("POD_NAMESPACE", "kwatch")
	e, _ := digestHarness(t, now)
	risk := func(ns, name string) detection.Finding {
		return detection.Finding{
			Entity: inventory.CoreID(kube.KindDeployment, ns, name),
			Reason: reasons.RiskSingleReplica, Advisory: true}
	}
	e.announcer.collect.SetAdvisories(func() []detection.Finding {
		return []detection.Finding{risk("kube-system", "coredns"),
			risk("kwatch", "kwatch"), risk("shop", "orders")}
	})
	got := e.announcer.collect.PendingRisks()
	if len(got) != 1 || got[0].Entity.Name != "orders" {
		t.Fatalf("pending risks = %v, want only orders", got)
	}
}

func TestEngineDigestKeepsSystemWorkloadNeverReady(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, _ := digestHarness(t, now)
	e.announcer.collect.SetAdvisories(func() []detection.Finding {
		return []detection.Finding{{
			Entity: inventory.CoreID(kube.KindDeployment, "kube-system",
				"coredns"),
			Reason: reasons.WorkloadNeverReady, Advisory: true}}
	})
	got := e.announcer.collect.PendingRisks()
	if len(got) != 1 {
		t.Fatalf("pending risks = %v, want the never-ready one", got)
	}
}
