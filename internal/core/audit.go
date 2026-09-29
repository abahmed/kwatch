package core

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/notice"
	"github.com/abahmed/kwatch/internal/problem"
)

var auditActions = map[problem.Action]audit.Action{
	problem.Announce: audit.ActionCreate,
	problem.Update:   audit.ActionUpdate,
	problem.Resolve:  audit.ActionResolved,
}

var tierNames = map[problem.Tier]string{
	problem.Silent: "silent", problem.Digest: "digest",
	problem.Notify: "notify", problem.Page: "page",
}

// AuditEntry records one decision and the message it produced.
func AuditEntry(
	d problem.Decision, m notice.Message, at time.Time,
) audit.Entry {
	p := d.Problem
	entry := audit.Entry{
		Timestamp: at, Action: auditActions[d.Action], Problem: p.ID,
		Namespace:      p.Root.Namespace,
		Reason:         strings.Join(m.Route.Reasons, ","),
		Severity:       m.Route.Severity,
		Root:           p.Root.String(),
		Tier:           tierNames[p.Tier],
		Revision:       p.Revision,
		AffectedCount:  len(p.Impact),
		CauseState:     causeState(p),
		Confidence:     m.Confidence,
		DecisionReason: d.Reason,
		ContentHash:    p.Digest,
	}
	if p.Cause != nil {
		entry.RootCause = p.Cause.Summary
	}
	return entry
}

// causeState classifies the explanation. A cause is only the failing
// object itself when it blames the one member entity and no change.
func causeState(p problem.Problem) string {
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
