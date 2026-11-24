package pipeline

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// FindingScope decides whether one finding is in scope.
type FindingScope interface {
	Allows(model inventory.Reader, sig detection.Finding) bool
}

// IncidentScope returns an InScope function: an incident is in scope when
// its announcement was delivered, or when any finding it explains, or any
// finding of its root, is. Remembering the announcement keeps a resolve in
// scope after every finding that put the incident there has cleared.
func IncidentScope(
	model inventory.Reader, scope FindingScope,
) func(incident.Incident) bool {
	return func(p incident.Incident) bool {
		if p.Scope == incident.ScopeIn {
			return true
		}
		for _, member := range p.Members {
			if scope.Allows(model, member) {
				return true
			}
		}
		if p.Cause == nil {
			return false
		}
		for _, root := range p.Cause.RootFindings {
			if scope.Allows(model, root) {
				return true
			}
		}
		return false
	}
}

// inScope drops decisions people do not want to hear about. The verdict
// on an announcement is remembered on the incident, so its updates and
// resolve are delivered even when no in-scope finding is left.
func (a *announcer) inScope(
	decisions []incident.Decision,
) []incident.Decision {
	if a.scope == nil {
		return decisions
	}
	var kept []incident.Decision
	for _, d := range decisions {
		if d, ok := a.judge(d); ok {
			kept = append(kept, d)
		}
	}
	return kept
}

// judge is one decision as inScope keeps it, and whether it does.
func (a *announcer) judge(d incident.Decision) (incident.Decision, bool) {
	in := a.scope(d.Incident)
	if in && d.Action == incident.Update &&
		d.Incident.Scope == incident.ScopeOut {
		// Nobody heard the announcement: the first message people
		// get about this incident must introduce it.
		d.Action = incident.Announce
	}
	if d.Action != incident.Announce {
		return d, in
	}
	if !in && a.waitsForScope(d.Incident) {
		return d, false
	}
	if a.incidents != nil {
		a.incidents.RecordScope(d.Incident.ID, in)
	}
	if !in {
		logWithheld(d.Incident)
	}
	return d, in
}

// logWithheld says, once per incident, that an announcement was dropped
// by the configured scope (namespaces, reasons, silences): without it a
// real failure people filtered out leaves no trace at all.
func logWithheld(p incident.Incident) {
	klog.InfoS("incident not announced: out of scope",
		"component", "pipeline", "operation", "scope",
		"incident", p.ID, "namespace", p.Root.Namespace,
		"kind", string(p.Root.Kind), "name", p.Root.Name,
		"tier", p.Tier.String())
}

// decisions are the decisions of one tick: those of the incident
// lifecycles, then the announcements of incidents that came back into
// scope, and the delay until the next lifecycle deadline.
func (a *announcer) decisions(
	now time.Time,
) ([]incident.Decision, time.Duration) {
	decisions, next := a.incidents.Tick(now)
	return append(decisions, a.backInScope(now, decisions)...), next
}

// rescopeEvery bounds how often backInScope asks the scope again about
// every unheard incident. The loop wakes many times a second under
// load, and each ask copies every out-of-scope failing incident and
// runs the scope on each member, while what the scope reads (a
// maintenance window, a silence, the configuration) changes far more
// rarely. A failure whose window ended is announced within this bound.
// The incidents waiting for a scope verdict (scope_pending.go) are
// asked on every tick: they are few, and their wait is short.
const rescopeEvery = 30 * time.Second

// backInScope asks the scope again about each failing incident whose
// announcement it dropped, and announces those it now allows. Without
// this a failure that outlives its maintenance window, or a silence,
// says nothing until its reminder is due, up to a week later: scope is
// otherwise only asked when an incident has something to say. An
// incident with a decision in this tick's list is left to inScope,
// which turns its update into the announcement: asked here as well, it
// would be announced twice, and it is asked on the next tick instead.
func (a *announcer) backInScope(
	now time.Time, decided []incident.Decision,
) []incident.Decision {
	only, due := a.rescopeDue(now)
	if a.scope == nil || !due {
		return nil
	}
	skip := make(map[string]bool, len(decided))
	for _, d := range decided {
		skip[d.Incident.ID] = true
	}
	var out []incident.Decision
	unheard := a.incidents.UnheardOf(only)
	a.pruneWaiting(unheard)
	skipped := false
	for _, p := range unheard {
		skipped = skipped || skip[p.ID]
		if skip[p.ID] || !a.scope(p) {
			a.decidedOut(p, skip[p.ID])
			continue
		}
		delete(a.waiting, p.ID)
		if d, ok := a.incidents.AnnounceUnheard(p.ID, now); ok {
			out = append(out, d)
		}
	}
	if only == nil && !skipped {
		a.rescopedAt = now
	}
	return out
}

// rescopeDue says which unheard incidents backInScope asks about at
// now: all of them (a nil only) once rescopeEvery passed since it last
// asked about all, otherwise only the ones waiting for a verdict. due
// is false when there is nothing to ask.
func (a *announcer) rescopeDue(
	now time.Time,
) (only func(id string) bool, due bool) {
	if a.rescopedAt.IsZero() ||
		!now.Before(a.rescopedAt.Add(rescopeEvery)) {
		return nil, true
	}
	if len(a.waiting) == 0 {
		return nil, false
	}
	return func(id string) bool { return a.waiting[id] }, true
}
