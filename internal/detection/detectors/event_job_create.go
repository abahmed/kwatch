package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// failedCreateReason is the event a controller records when the API
// server refuses a pod it tried to create.
const failedCreateReason = "FailedCreate"

// jobCreateOvercome reports a FailedCreate note of a Job that the Job
// got past: it completed, or it has a pod created after the note. The
// Job controller retries a refused create within seconds, so one
// refusal (a webhook that timed out once) is over by the time anyone
// reads about it. A Job that failed, or that still has no pod, keeps
// the note.
func jobCreateOvercome(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	if e.ID.Kind != kube.KindJob || note.Reason != failedCreateReason ||
		ctx.Model == nil {
		return false
	}
	if status, _, _ := condition(e, "Complete"); status == "True" {
		return true
	}
	for _, id := range ctx.Model.Related(
		e.ID, inventory.OwnedBy, inventory.Incoming,
	) {
		pod, ok := ctx.Model.Entity(id)
		if ok && id.Kind == kube.KindPod &&
			timestamp(pod, kube.AttrCreated).After(note.At) {
			return true
		}
	}
	return false
}
