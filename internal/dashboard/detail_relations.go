package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// IncidentRelations is how far an incident reaches and how it ties to
// the other open incidents. Each part is absent without data.
type IncidentRelations struct {
	// CausedBy are the open incidents this one is caused by, and
	// Causing the ones it causes (incident.Related), at most five each.
	CausedBy []RelatedIncident `json:"causedBy,omitempty"`
	Causing  []RelatedIncident `json:"causing,omitempty"`
	// BlastRadius says in one line what the incident affects now, or
	// at its widest once nothing fails: "Affects 2 services, 1 ingress
	// and 3 of 4 pods of payments".
	BlastRadius string `json:"blastRadius,omitempty"`
}

// RelatedIncident is one incident of CausedBy or Causing.
type RelatedIncident struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	State    string `json:"state"`
}

// addRelations fills d's relations from p. The incident manager filled
// p's related lists when it handed p out.
func addRelations(d *IncidentDetail, p incident.Incident) {
	d.CausedBy = toRelated(p.CausedBy)
	d.Causing = toRelated(p.Causing)
	radius := p.Radius
	if p.State == incident.Resolved || radius.Summary() == "" {
		radius = p.Journal.Widest
	}
	d.BlastRadius = radius.Summary()
}

func toRelated(in []incident.Related) []RelatedIncident {
	if len(in) == 0 {
		return nil
	}
	out := make([]RelatedIncident, 0, len(in))
	for _, r := range in {
		out = append(out, RelatedIncident{
			ID: r.ID, Title: redact.Evidence(r.Title),
			Severity: r.Tier.String(), State: r.State.String(),
		})
	}
	return out
}
