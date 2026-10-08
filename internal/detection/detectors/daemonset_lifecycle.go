package detectors

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A DaemonSet pod is part of its node: it starts when the node joins and
// ends when the node goes. A pod that fails, or is not ready, on a node
// that is leaving, not ready, gone or just joined is that node's
// lifecycle, not a problem of the DaemonSet. Only a failure on a settled,
// healthy node counts.

// nodeInTransition reports whether the node is part of node churn: it is
// gone from the model, not ready, cordoned, being deleted, or younger
// than kube.BootWindow. A young node asks for a recheck when it turns
// old, so what still fails then is reported.
func nodeInTransition(ctx detection.Context, id inventory.EntityID) bool {
	if ctx.Model == nil {
		return false
	}
	node, ok := ctx.Model.Entity(id)
	if !ok {
		return ctx.Synced(kube.KindNode)
	}
	if remaining := kube.NodeYoungRemaining(ctx.Model, id,
		ctx.Now); remaining > 0 {
		ctx.RecheckAfter(remaining)
		return true
	}
	if known, ok := node.Attribute(kube.AttrReady); ok {
		if ready, isBool := known.Value.AsBool(); isBool && !ready {
			return true
		}
	}
	return flag(node, kube.AttrUnschedulable) || flag(node, kube.AttrDeleting)
}

// daemonNodeRef finds the node a "FailedDaemonPod" event names: the
// DaemonSet controller says "Found failed daemon pod <ns>/<pod> on node
// <node>, will try to kill it".
var daemonNodeRef = regexp.MustCompile(`\bon node ([^\s,]+)`)

// daemonNodeNoise reports a FailedDaemonPod event of a DaemonSet whose
// pod failed on a node in transition. The event names its node; without
// one, the DaemonSet's own pods that are not ready decide, and only when
// every one of them sits on a node in transition.
func daemonNodeNoise(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	if e.ID.Kind != kube.KindDaemonSet || note.Reason != "FailedDaemonPod" ||
		ctx.Model == nil {
		return false
	}
	if match := daemonNodeRef.FindStringSubmatch(note.Message); match != nil {
		return nodeInTransition(ctx, inventory.CoreID(kube.KindNode, "",
			match[1]))
	}
	return daemonGapsInTransition(ctx, e)
}

// daemonGapsInTransition reports whether the DaemonSet has pods that are
// not ready and every one of them runs on a node in transition.
func daemonGapsInTransition(
	ctx detection.Context, e inventory.Entity,
) bool {
	if e.ID.Kind != kube.KindDaemonSet || ctx.Model == nil {
		return false
	}
	found := false
	for _, id := range ctx.Model.Related(e.ID, inventory.OwnedBy,
		inventory.Incoming) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || id.Kind != kube.KindPod || flag(pod, kube.AttrReady) {
			continue
		}
		nodes := ctx.Model.Related(id, inventory.RunsOn, inventory.Outgoing)
		if len(nodes) == 0 {
			return false
		}
		for _, node := range nodes {
			if !nodeInTransition(ctx, node) {
				return false
			}
		}
		found = true
	}
	return found
}
