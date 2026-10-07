package pipeline

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/status"
)

// Status builds the /status report from state the engine already holds:
// the incidents, the active findings and the model. It is safe to call
// from any goroutine while the engine runs, and it starts no work.
func (e *Engine) Status() status.Report {
	now := e.deps.Clock.Now()
	findings := e.published.all()
	return status.Report{
		At:           now,
		Cluster:      e.deps.Writer.Cluster,
		Problems:     e.problems(now),
		ControlPlane: status.ControlPlane(e.deps.Model, findings),
		Upgrade:      status.Assess(findings, e.deps.Model),
		Zones:        status.ZoneHealth(e.deps.Model, findings),
	}
}

// Readiness is the upgrade-readiness answer for the digest.
func (e *Engine) Readiness() status.Readiness {
	return status.Assess(e.published.all(), e.deps.Model)
}

// problems lists the open incidents people are told about, loudest
// first.
func (e *Engine) problems(now time.Time) status.Problems {
	var open []incident.Incident
	for _, p := range e.deps.Incidents.Incidents() {
		if p.State == incident.Resolved || p.Tier == incident.Silent {
			continue
		}
		if e.deps.InScope != nil && !e.deps.InScope(p) {
			continue
		}
		open = append(open, p)
	}
	sort.SliceStable(open, func(i, j int) bool {
		if open[i].Tier != open[j].Tier {
			return open[i].Tier > open[j].Tier
		}
		return open[i].Opened.Before(open[j].Opened)
	})
	out := status.Problems{Open: len(open)}
	for i, p := range open {
		if i == status.MaxProblems {
			out.More = len(open) - i
			break
		}
		out.Items = append(out.Items, e.problem(p, now))
	}
	return out
}

// problem is one incident as /status shows it. The cause line is the
// lead of the message people got, so the two never disagree.
func (e *Engine) problem(p incident.Incident, now time.Time) status.Problem {
	msg := e.deps.Writer.Write(
		incident.Decision{Action: incident.Update, Incident: p}, now)
	return status.Problem{
		ID: p.ID, Tier: p.Tier.String(), State: p.State.String(),
		Root:       string(p.Root.Kind) + " " + status.EntityName(p.Root),
		OpenedAt:   p.Opened,
		AgeSeconds: int64(now.Sub(p.Opened).Seconds()),
		Cause:      redact.Evidence(msg.Title),
	}
}
