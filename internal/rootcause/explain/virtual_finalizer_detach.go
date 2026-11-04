package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// handlerCannotFinish reports an object whose deletion its finalizer
// holds because the handler runs and fails, as the object's own finding
// says: a volume attachment whose detach returned an error, or whose
// volume or node no longer exists. That is true of any storage driver:
// its attacher removes the finalizer only after a detach that cannot
// succeed any more. Nothing is missing or stopped, so the finalizer is
// not the cause; the object is its own root, and its finding says what
// is gone and that the finalizer must be removed by hand.
func (v *view) handlerCannotFinish(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if !f.Mode.Within(detection.ModeVolumeDetachFailed) {
			continue
		}
		for _, e := range f.Evidence {
			if e.Label == detection.EvidenceAttachGone ||
				e.Label == detection.EvidenceDetachError {
				return true
			}
		}
	}
	return false
}
