package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restartingOwners are the controllers whose pods always restart their
// containers (restartPolicy Always, which the API enforces for them): a
// container of theirs that exited is about to start again, not done.
var restartingOwners = map[inventory.Kind]bool{
	kube.KindReplicaSet: true, kube.KindStatefulSet: true,
	kube.KindDaemonSet: true, kube.KindDeployment: true,
}

// unreadyOnlyByFinished reports a running pod that is not ready only
// because some of its containers finished their work: every container
// that is not ready exited 0 and at least one other is ready. A Job
// pod with a helper that ends before the main container looks like
// this until the pod completes, and Kubernetes reports it not ready
// (ContainersNotReady) the whole time. Init and debug containers are
// not counted.
func unreadyOnlyByFinished(
	ctx detection.Context, pod inventory.Entity,
) bool {
	if ctx.Model == nil || ownedByRestarting(ctx, pod.ID) {
		return false
	}
	finished, ready := 0, 0
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.PartOf, inventory.Incoming,
	) {
		c, ok := ctx.Model.Entity(id)
		if !ok || id.Kind != kube.KindContainer ||
			flag(c, kube.AttrInit) || flag(c, kube.AttrEphemeral) {
			continue
		}
		switch {
		case flag(c, kube.AttrReady):
			ready++
		case finishedCleanly(c):
			finished++
		default:
			return false
		}
	}
	return finished > 0 && ready > 0
}

// finishedCleanly is a container that ended with exit code 0.
func finishedCleanly(c inventory.Entity) bool {
	code, known := number(c, kube.AttrExitCode)
	return text(c, kube.AttrState) == "terminated" && known && code == 0
}

func ownedByRestarting(
	ctx detection.Context, pod inventory.EntityID,
) bool {
	for _, owner := range ctx.Model.Related(
		pod, inventory.OwnedBy, inventory.Outgoing,
	) {
		if restartingOwners[owner.Kind] {
			return true
		}
	}
	return false
}
