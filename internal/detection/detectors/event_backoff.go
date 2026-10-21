package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backoffLife is how long a controller's note outlives EventWindow while
// the controller is still retrying. A controller backs off a refused
// create for longer than EventWindow, so between two tries the event is
// older than the window although the failure goes on.
const backoffLife = EventWindow

// maxNoteLife is the longest a note can count as current.
const maxNoteLife = EventWindow + backoffLife

// noteDeadline is when the note stops counting as current: EventWindow
// after it was written, and for a controller that still lacks pods
// because it cannot create them, one more window, the time its backoff
// takes to come round to another try.
func noteDeadline(e inventory.Entity, note inventory.Note) time.Time {
	life := EventWindow
	if controllerBackingOff(e, note) {
		life = maxNoteLife
	}
	return note.At.Add(life)
}

// controllerBackingOff reports a FailedCreate note of a controller that
// is still short of its pods, so the controller is waiting out a
// backoff before it tries again.
func controllerBackingOff(e inventory.Entity, note inventory.Note) bool {
	if note.Reason != "FailedCreate" {
		return false
	}
	desired, known := desiredReplicas(e)
	ready, _ := number(e, kube.AttrReadyReplicas)
	return known && desired > ready
}
