package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nodeGoneModes is ModeDeleted for a node that no longer exists, seen
// from a pod stuck terminating on it: the pod's record names a node
// whose object is gone, so nothing will finish the pod. A node that
// kwatch does not watch yet may only look missing, so the kind must have
// synced.
func (v *view) nodeGoneModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if id.Kind != kube.KindNode || link != LinkRunsOn ||
		effect.Kind != kube.KindPod ||
		v.s.Model.Exists(id) || !v.s.verifiable(kube.KindNode) ||
		!v.hasMode(effect, []detection.Mode{detection.ModeStuckDeleting}) {
		return nil
	}
	return pseudoFailing(ModeDeleted)
}
