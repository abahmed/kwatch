package detection

import (
	"slices"

	"github.com/abahmed/kwatch/internal/inventory"
)

// SyncedFor reports whether kind is fully watched, like Synced, for a
// judgement that only some of a detector's findings depend on: a
// reference found missing, which counts only when its kind is synced.
// When the kind is not synced, only the named reasons are unsure: their
// earlier findings are kept (Evaluation.Keeping), while every other
// finding of the detector is judged, raised and cleared as usual. The
// kind is noted so the entity is evaluated again once it syncs.
//
// Call it only once the judgement needs it, after the referenced object
// was found absent: a detector that asks up front makes its reasons
// unsure on every evaluation, whether anything is missing or not.
func (c Context) SyncedFor(
	kind inventory.Kind, reasons ...string,
) bool {
	if c.synced == nil || c.synced(kind) {
		return true
	}
	c.noteUnsynced(kind)
	if c.unsure != nil {
		for _, reason := range reasons {
			if !slices.Contains(*c.unsure, reason) {
				*c.unsure = append(*c.unsure, reason)
			}
		}
	}
	return false
}

// noteUnsynced records kind as needed but not synced.
func (c Context) noteUnsynced(kind inventory.Kind) {
	if c.unsynced != nil && !slices.Contains(*c.unsynced, kind) {
		*c.unsynced = append(*c.unsynced, kind)
	}
}

// Unsure reports whether the evaluation leaves anything unjudged: a
// detector panicked or could not run, or a reason could not be judged.
func (ev Evaluation) Unsure() bool {
	return len(ev.Panicked) > 0 || len(ev.UnsyncedBy) > 0 ||
		len(ev.UnsureReasons) > 0
}
