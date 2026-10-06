package pipeline

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

var auditActions = map[incident.Action]audit.Action{
	incident.Announce: audit.ActionCreate,
	incident.Update:   audit.ActionUpdate,
	incident.Resolve:  audit.ActionResolved,
}

var tierNames = map[incident.Tier]string{
	incident.Silent: "silent", incident.Digest: "digest",
	incident.Notify: "notify", incident.Page: "page",
}

// AuditEntry records one decision and the message it produced.
func AuditEntry(
	d incident.Decision, m notification.Message, at time.Time,
) audit.Entry {
	p := d.Incident
	entry := audit.Entry{
		Timestamp: at, Action: auditActions[d.Action], Incident: p.ID,
		Namespace:      p.Root.Namespace,
		Reason:         strings.Join(m.Route.Reasons, ","),
		Severity:       m.Route.Severity,
		Root:           p.Root.String(),
		Tier:           tierNames[p.Tier],
		Revision:       p.Revision,
		AffectedCount:  len(p.Impact),
		CauseState:     causeState(p),
		Confidence:     m.Confidence,
		DecisionReason: string(d.Reason),
		ContentHash:    contentHash(d),
		Previous:       p.Previous,
		Delivery:       m.Carrier,
		DeliveryNote:   m.CarrierNote,
		Considered:     p.Considered,
	}
	switch {
	case m.PagingOnly:
		entry.Delivery = "paging"
	case m.SkipPaging && entry.Delivery == "":
		entry.Delivery = "chat"
	}
	if m.Listed != nil {
		entry.Opened, entry.Resolved = m.Listed.Opened, m.Listed.Resolved
		entry.Risks, entry.Items = m.Listed.Risks, m.Listed.Items
	}
	if p.ID == "" {
		summaryEntry(&entry, m)
	}
	if p.Cause != nil {
		entry.RootCause = p.Cause.Summary
	}
	return entry
}

// summaryItems bounds the incidents a roll-up entry names.
const summaryItems = 10

// summaryEntry makes the entry of a message that is about several
// incidents (a roll-up, a startup summary, a digest) say so: it has no
// incident, so it names no root, and a roll-up lists its incidents.
func summaryEntry(entry *audit.Entry, m notification.Message) {
	if entry.Action == "" {
		entry.Action = audit.ActionSummary
	}
	entry.Namespace, entry.Root = "", ""
	if len(m.Members) > 0 && entry.Items == nil {
		entry.Items = m.Members[:min(len(m.Members), summaryItems)]
		entry.Opened = len(m.Members)
	}
}

// contentHash fingerprints what the message says. A reminder states how
// long the incident has been open, so it says something new each time
// even though the incident's fingerprint has not changed.
func contentHash(d incident.Decision) string {
	p := d.Incident
	if d.Reason == incident.ReasonReminder {
		return p.Digest + "@" + p.Reminded.UTC().Format(time.RFC3339)
	}
	return p.Digest
}

// causeState classifies the explanation. A cause is only the failing
// object itself when it blames the one member entity and no change.
func causeState(p incident.Incident) string {
	if p.Cause == nil {
		return audit.CauseUnknown
	}
	if p.Cause.Change != nil {
		return audit.CauseKnown
	}
	for _, member := range p.Members {
		if member.Entity != p.Cause.Root {
			return audit.CauseKnown
		}
	}
	return audit.CauseSelf
}
