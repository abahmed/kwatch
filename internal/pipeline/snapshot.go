package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// snapshot is what root cause explains at now: the model, the active
// findings and what the edition withholds.
func (e *Engine) snapshot(now time.Time) explain.Snapshot {
	s := explain.NewSnapshot(e.deps.Model, e.activeFindings(),
		e.deps.Synced, now)
	s.Started = e.started
	s.NoSharedFactors = !feature.OrCommunity(
		e.deps.Entitlements).Has(feature.SharedFactors)
	return s
}

// countWithheld counts the incidents a withheld shared factor would
// explain. Only their opaque IDs are hashed; nothing is named.
func (e *Engine) countWithheld(
	now time.Time, decisions []incident.Decision,
) {
	for _, d := range decisions {
		if d.Incident.ProCause {
			e.deps.Withheld.Record(now, feature.WithheldSharedCauses,
				d.Incident.ID)
		}
	}
}
