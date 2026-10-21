package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// deletionEventWindow is how far back the Warning events of a stuck
// deletion are read: Kubernetes keeps an event for an hour, and a
// controller that cannot finish a deletion repeats its complaint with
// back-off, so the latest one can be many minutes old.
const deletionEventWindow = time.Hour

// lastWarning is the message of the newest Warning event of e within
// deletionEventWindow. The controller that holds the deletion may be
// one kwatch cannot see; what it wrote is then the only reason there is.
// The finding is looked at again when the event ages out, so it never
// quotes a complaint that is no longer current.
func lastWarning(
	ctx detection.Context, e inventory.Entity,
) (string, bool) {
	if ctx.Model == nil {
		return "", false
	}
	var newest inventory.Note
	found := false
	for _, note := range ctx.Model.Notes(
		e.ID, ctx.Now.Add(-deletionEventWindow)) {
		if note.Warning && note.Message != "" &&
			(!found || note.At.After(newest.At)) {
			newest, found = note, true
		}
	}
	if !found {
		return "", false
	}
	ctx.RecheckAfter(
		newest.At.Add(deletionEventWindow).Sub(ctx.Now) + time.Nanosecond)
	return newest.Message, true
}
