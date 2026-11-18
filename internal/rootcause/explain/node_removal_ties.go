package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// formerRelations is the part of the inventory that keeps what a
// deleted entity was joined to. The production model has it; small
// test models may not.
type formerRelations interface {
	FormerRelated(
		id inventory.EntityID, relation inventory.RelationType,
	) []inventory.EntityID
}

// removalTouches reports whether a node's removal can be why a waiting
// pod waits: the pod ran on that node, it replaces a pod of its owner
// that ran there, or it is not placed yet and the scheduler names
// missing capacity, which any removed node had. A placed pod that never
// ran there, nor replaces a pod that did, waits for something of its
// own node (its sandbox, image or volume), which no removal elsewhere
// explains, whatever its timing.
func (v *view) removalTouches(pod, node inventory.EntityID) bool {
	if v.runsOn(pod, node) || v.ranOn(pod, node) {
		return true
	}
	for _, peer := range v.removedPeers(pod) {
		if v.ranOn(peer, node) {
			return true
		}
	}
	if len(v.s.Model.Related(pod, inventory.RunsOn,
		inventory.Outgoing)) > 0 {
		return false
	}
	reason, ok := v.schedulerVerdict(pod)
	return ok && strings.HasPrefix(reason, "Insufficient")
}

// ranOn reports a pod deleted in the window that ran on node when it
// was deleted. A pod of a StatefulSet keeps its name, so the pod itself
// may exist again.
func (v *view) ranOn(pod, node inventory.EntityID) bool {
	former, ok := v.s.Model.(formerRelations)
	if !ok || !v.deleted(pod) {
		return false
	}
	return slices.Contains(former.FormerRelated(pod, inventory.RunsOn),
		node)
}

// removedPeers are the pods deleted in the window that had an owner of
// pod: the pods it may replace.
func (v *view) removedPeers(pod inventory.EntityID) []inventory.EntityID {
	owners := v.s.Model.Related(pod, inventory.OwnedBy, inventory.Outgoing)
	former, ok := v.s.Model.(formerRelations)
	if len(owners) == 0 || !ok {
		return nil
	}
	var out []inventory.EntityID
	for _, gone := range v.removedOf(kube.KindPod) {
		for _, owner := range former.FormerRelated(gone,
			inventory.OwnedBy) {
			if slices.Contains(owners, owner) {
				out = append(out, gone)
				break
			}
		}
	}
	return out
}
