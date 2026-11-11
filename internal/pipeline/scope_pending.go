package pipeline

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// A scope can refuse a finding only for now: the namespace label
// selector waits a little for a Namespace that is not in the model yet.
// That refusal is no verdict. An announcement it drops is not logged as
// out of scope: the incident stays unheard, as one out of scope does,
// and backInScope asks again on the next tick. Once the scope decides,
// it is announced, or logged as out of scope then.

// PendingScope tells a refusal that is only for now.
type PendingScope interface {
	Pending(model inventory.Reader, sig detection.Finding) bool
}

// IncidentPending returns a ScopePending function: an incident's
// refusal is only for now when any finding it explains, or any finding
// of its root, waits for the scope.
func IncidentPending(
	model inventory.Reader, scope PendingScope,
) func(incident.Incident) bool {
	return func(p incident.Incident) bool {
		for _, member := range p.Members {
			if scope.Pending(model, member) {
				return true
			}
		}
		if p.Cause == nil {
			return false
		}
		for _, root := range p.Cause.RootFindings {
			if scope.Pending(model, root) {
				return true
			}
		}
		return false
	}
}

// waitsForScope keeps the announcement of p, which the scope refused
// only for now, unheard without a verdict: no "out of scope" log. It
// reports false when the refusal is a verdict.
func (a *announcer) waitsForScope(p incident.Incident) bool {
	if a.pending == nil || !a.pending(p) {
		return false
	}
	if a.incidents != nil {
		// Unheard, so that backInScope asks again; not withheld.
		a.incidents.RecordScopeWaiting(p.ID)
	}
	if a.waiting == nil {
		a.waiting = map[string]bool{}
	}
	a.waiting[p.ID] = true
	return true
}

// decidedOut logs, once, the verdict on a waiting incident that the
// scope now refuses for good. A refusal still pending keeps waiting;
// an incident skipped this tick is asked on the next.
func (a *announcer) decidedOut(p incident.Incident, skipped bool) {
	if skipped || !a.waiting[p.ID] ||
		(a.pending != nil && a.pending(p)) {
		return
	}
	delete(a.waiting, p.ID)
	if a.incidents != nil {
		a.incidents.RecordScopeDecided(p.ID)
	}
	logWithheld(p)
}

// pruneWaiting forgets the waiting incidents that are no longer
// unheard: announced, resolved or gone.
func (a *announcer) pruneWaiting(unheard []incident.Incident) {
	if len(a.waiting) == 0 {
		return
	}
	live := make(map[string]bool, len(unheard))
	for _, p := range unheard {
		live[p.ID] = true
	}
	for id := range a.waiting {
		if !live[id] {
			delete(a.waiting, id)
		}
	}
}
