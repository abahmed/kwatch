package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// killFoldWindow is how soon after a container's last failed liveness
// probe a stop event still belongs to the kill that probe caused: the
// kubelet kills at once and runs the preStop hook within the grace
// period.
const killFoldWindow = time.Minute

// partOfLivenessKill reports a stop event (detection.IsKillSideEffect)
// that the kubelet wrote while killing the container for its failed
// liveness probe: the liveness finding already tells that story, and a
// failed preStop hook of a container that was being killed anyway must
// not become a failure of its own. The kubelet keeps one Unhealthy
// event per probe and moves its time on with every failure, so a
// liveness failure after the stop event (the next run failing too)
// also counts.
func partOfLivenessKill(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	if !detection.IsKillSideEffect(note.Reason) {
		return false
	}
	since := note.At.Add(-killFoldWindow)
	for _, other := range ctx.Model.Notes(e.ID, since) {
		if other.Reason == "Unhealthy" && other.Topic == "liveness" {
			return true
		}
	}
	return false
}
