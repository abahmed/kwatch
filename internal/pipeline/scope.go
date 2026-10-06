package pipeline

import (
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
		in := a.scope(d.Incident)
		if in && d.Action == incident.Update &&
			d.Incident.Scope == incident.ScopeOut {
			// Nobody heard the announcement: the first message people
			// get about this incident must introduce it.
			d.Action = incident.Announce
		}
		if d.Action == incident.Announce && a.incidents != nil {
			a.incidents.RecordScope(d.Incident.ID, in)
		}
		if in {
			kept = append(kept, d)
		}
	}
	return kept
}
