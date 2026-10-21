package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// joinSlack is how long after the node was created its Ready condition
// may last have changed for the node to count as never Ready. The
// kubelet posts its first status when it registers, and the node
// controller marks a node that stays silent Unknown within about a
// minute. A Ready condition that changed later than this was Ready in
// between.
const joinSlack = 2 * time.Minute

// joinedNever reports a node that has not been Ready since it joined,
// and when it joined. A node that kwatch saw Ready, or whose Ready
// condition changed long after it was created, was Ready once: its
// NotReady is a loss, not a slow start.
func joinedNever(
	e inventory.Entity, status string, since time.Time,
) (time.Time, bool) {
	created := timestamp(e, kube.AttrCreated)
	if created.IsZero() || status == "True" {
		return time.Time{}, false
	}
	if attribute, ok := e.Attribute(kube.AttrReady); ok &&
		len(attribute.Flips) > 0 {
		return time.Time{}, false
	}
	if !since.IsZero() && since.Sub(created) > joinSlack {
		return time.Time{}, false
	}
	return created, true
}

// joinHold holds back the report of a node that has not been Ready
// since it joined until kube.BootWindow has passed since its creation:
// a node still pulling images and starting its network plugin is slow,
// not lost. It asks to look again when the window ends, and reports
// whether it is still holding. The second result is how long ago the
// node joined, for the report that follows the window, or zero for a
// node that was Ready once.
func joinHold(
	ctx detection.Context, e inventory.Entity, status string,
	since time.Time,
) (holding bool, neverReady time.Duration) {
	joined, never := joinedNever(e, status, since)
	if !never {
		return false, 0
	}
	age := ctx.Now.Sub(joined)
	if remaining := kube.BootWindow - age; remaining > 0 {
		ctx.RecheckAfter(remaining)
		return true, 0
	}
	return false, age
}

// neverReadySummary words a NotReady report for a node that never
// became Ready since it joined.
func neverReadySummary(age time.Duration) string {
	return "Node has not been Ready since it joined " +
		format.Duration(age) + " ago"
}
