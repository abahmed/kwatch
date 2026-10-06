package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func auditIncident(
	cause *rootcause.CauseRecord, members ...inventory.EntityID,
) incident.Incident {
	p := incident.Incident{
		ID: "p1", Root: inventory.EntityID{
			Kind: "deployment", Namespace: "shop", Name: "api"},
		Cause: cause, Tier: incident.Page, Revision: 2, Digest: "h1",
		Members: map[detection.Key]detection.Finding{},
		Impact:  []inventory.EntityID{{Kind: "service", Name: "api"}},
	}
	for _, id := range members {
		s := detection.Finding{Entity: id, Reason: "CrashLoopBackOff"}
		p.Members[s.Key()] = s
	}
	return p
}

func TestAuditEntryRecordsDecision(t *testing.T) {
	pod := inventory.EntityID{Kind: "pod", Namespace: "shop", Name: "api-1"}
	p := auditIncident(&rootcause.CauseRecord{
		Root: inventory.EntityID{Kind: "deployment", Namespace: "shop",
			Name: "api"},
		Summary: "the rollout that changed the image",
	}, pod)
	p.Previous = "p0"
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	entry := AuditEntry(incident.Decision{
		Action: incident.Announce, Incident: p, Reason: "settled",
	}, notification.Message{
		Confidence: "high",
		Route: notification.Route{Reasons: []string{"CrashLoopBackOff"},
			Severity: "critical"},
	}, at)

	assert.Equal(t, audit.Entry{
		Timestamp: at, Action: audit.ActionCreate, Incident: "p1",
		Namespace: "shop", Reason: "CrashLoopBackOff",
		Severity: "critical", Root: "deployment/shop/api", Tier: "page",
		Revision: 2, AffectedCount: 1, CauseState: audit.CauseKnown,
		RootCause:      "the rollout that changed the image",
		Confidence:     "high",
		DecisionReason: "settled", ContentHash: "h1", Previous: "p0",
	}, entry)
}

// The entry says how a decision reaches people when not as a message of
// its own.
func TestAuditEntryRecordsDelivery(t *testing.T) {
	pod := inventory.EntityID{Kind: "pod", Namespace: "shop", Name: "api-1"}
	p := auditIncident(nil, pod)
	d := incident.Decision{Action: incident.Announce, Incident: p}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	own := AuditEntry(d, notification.Message{}, at)
	carried := AuditEntry(d, notification.Message{Carrier: "digest"}, at)
	paging := AuditEntry(d, notification.Message{PagingOnly: true}, at)

	assert.Empty(t, own.Delivery)
	assert.Equal(t, "digest", carried.Delivery)
	assert.Equal(t, "paging", paging.Delivery)
}

func TestAuditCauseState(t *testing.T) {
	pod := inventory.EntityID{Kind: "pod", Namespace: "shop", Name: "api-1"}
	self := &rootcause.CauseRecord{Root: pod}
	change := &rootcause.CauseRecord{Root: pod, Change: &inventory.Change{}}

	assert.Equal(t, audit.CauseUnknown, causeState(auditIncident(nil, pod)))
	assert.Equal(t, audit.CauseSelf, causeState(auditIncident(self, pod)))
	assert.Equal(t, audit.CauseKnown, causeState(auditIncident(change, pod)))
}

func TestAuditReminderHashDiffersFromTheAnnouncement(t *testing.T) {
	p := auditIncident(nil)
	p.Reminded = time.Date(2026, 9, 29, 16, 0, 0, 0, time.UTC)
	at := p.Reminded
	update := incident.Decision{Action: incident.Update, Incident: p}
	reminder := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonReminder}
	plain := AuditEntry(update, notification.Message{}, at)
	again := AuditEntry(reminder, notification.Message{}, at)
	assert.Equal(t, "h1", plain.ContentHash)
	assert.NotEqual(t, plain.ContentHash, again.ContentHash)
}
