package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// observeResult hands what the detectors found for id to the tracker. A
// detector that panicked is unknown, not silent: the entity keeps the
// findings that detector raised before, while the other detectors'
// results apply as usual. The registry asks for another evaluation.
//
// Findings held over a kind kwatch cannot verify are noted with the
// incidents, which say so once the hold lasts (incident.NoteHeld).
func (e *Engine) observeResult(
	now time.Time, id inventory.EntityID, result detection.Evaluation,
) []detection.Transition {
	current := result.Findings
	// An evaluation that could not run fully proves nothing cleared: a
	// detector panicked, or one needed a kind not synced yet. A gone
	// entity's findings still clear.
	unsynced := len(result.UnsyncedBy) > 0 && e.deps.Model.Exists(id)
	if len(result.Panicked) > 0 || unsynced {
		current = result.Keeping(e.tracker.Active(id))
	}
	e.deps.Incidents.NoteHeld(id,
		unsynced && len(current) > len(result.Findings), now)
	return e.tracker.Observe(id, current)
}
