package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/notice"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

func auditProblem(
	cause *reason.Hypothesis, members ...knowledge.EntityID,
) problem.Problem {
	p := problem.Problem{
		ID: "p1", Root: knowledge.EntityID{
			Kind: "deployment", Namespace: "shop", Name: "api"},
		Cause: cause, Tier: problem.Page, Revision: 2, Digest: "h1",
		Members: map[signal.Key]signal.Signal{},
		Impact:  []knowledge.EntityID{{Kind: "service", Name: "api"}},
	}
	for _, id := range members {
		s := signal.Signal{Entity: id, Reason: "CrashLoopBackOff"}
		p.Members[s.Key()] = s
	}
	return p
}

func TestAuditEntryRecordsDecision(t *testing.T) {
	pod := knowledge.EntityID{Kind: "pod", Namespace: "shop", Name: "api-1"}
	p := auditProblem(&reason.Hypothesis{
		Root: knowledge.EntityID{Kind: "deployment", Namespace: "shop",
			Name: "api"},
		Summary: "the rollout that changed the image",
	}, pod)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	entry := AuditEntry(problem.Decision{
		Action: problem.Announce, Problem: p, Reason: "settled",
	}, notice.Message{
		Confidence: "high",
		Route: notice.Route{Reasons: []string{"CrashLoopBackOff"},
			Severity: "critical"},
	}, at)

	assert.Equal(t, audit.Entry{
		Timestamp: at, Action: audit.ActionCreate, Problem: "p1",
		Namespace: "shop", Reason: "CrashLoopBackOff",
		Severity: "critical", Root: "deployment/shop/api", Tier: "page",
		Revision: 2, AffectedCount: 1, CauseState: audit.CauseKnown,
		RootCause:      "the rollout that changed the image",
		Confidence:     "high",
		DecisionReason: "settled", ContentHash: "h1",
	}, entry)
}

func TestAuditCauseState(t *testing.T) {
	pod := knowledge.EntityID{Kind: "pod", Namespace: "shop", Name: "api-1"}
	self := &reason.Hypothesis{Root: pod}
	change := &reason.Hypothesis{Root: pod, Change: &knowledge.Change{}}

	assert.Equal(t, audit.CauseUnknown, causeState(auditProblem(nil, pod)))
	assert.Equal(t, audit.CauseSelf, causeState(auditProblem(self, pod)))
	assert.Equal(t, audit.CauseKnown, causeState(auditProblem(change, pod)))
}
