package explain

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeDeleted is the deletion of the object, a finer ModeChanged.
const ModeDeleted detection.Mode = "Changed.Deleted"

// LinkBlocks is the link from an effect to what keeps it from
// finishing, as a budget keeps a drain from evicting a pod.
const LinkBlocks LinkType = "blocks"

// nodeLifecycleRows cover nodes leaving the cluster (ADR 0010 rule 18):
// a scale-in, a replacement or a spot interruption takes a node away,
// its pods go with it and their replacements may find no room.
var nodeLifecycleRows = []Row{
	{
		// The pods that ran on a removed node.
		Name: "node-removed",
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeDeleted}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeNotReady, detection.ModeStatusUnknown,
			detection.ModeEvicted, detection.ModeFailed,
			detection.ModeStuckDeleting, detection.ModeProbe),
		Prior: 0.8,
	},
	{
		// A budget as strict as its replica count (minAvailable 1 of
		// 1) lets no pod leave a cordoned node: the drain waits on it.
		Name: "budget-blocks-drain",
		Cause: Side{Kind: kube.KindPDB,
			Modes: []detection.Mode{detection.ModeBudgetBlocksDrain}},
		Link: LinkBlocks,
		Effect: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{detection.ModeDraining}},
		Prior: 0.8,
	},
	{
		// Replacements that wait for the capacity the removed node
		// had. Stronger than the scheduler's own reason: "Insufficient
		// cpu" is true, but it is true because a node went away.
		Name: "node-removed-capacity",
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeDeleted}},
		Link:   LinkSchedules,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable),
		Prior:  0.75,
	},
}

// removedNodeHops lead from a pod that waits for a node to the nodes
// removed just before it started waiting: it is a replacement of a pod
// that ran there, or wants the room that node had.
func (v *view) removedNodeHops(pod inventory.EntityID) []hop {
	waiting := v.earliestSince([]inventory.EntityID{pod})
	if !waiting.ok || !v.hasMode(pod, []detection.Mode{
		detection.ModePending, detection.ModeUnschedulable}) {
		return nil
	}
	var out []hop
	for _, node := range v.removedNodes() {
		if v.replacedAfter(node, waiting.t) {
			out = append(out, hop{link: LinkSchedules, to: node})
		}
	}
	return out
}

// replacedAfter reports whether a pod that began waiting at t did so
// within NodeReplacementWindow after the node was deleted.
func (v *view) replacedAfter(node inventory.EntityID, t time.Time) bool {
	for _, change := range v.changesOf(node) {
		if change.Deleted && !t.Before(change.At.Add(-TemporalSlack)) &&
			!t.After(change.At.Add(NodeReplacementWindow)) {
			return true
		}
	}
	return false
}

// removedNodes lists the nodes deleted in the window, once per solve.
func (v *view) removedNodes() []inventory.EntityID {
	if v.removed != nil {
		return *v.removed
	}
	found := map[inventory.EntityID]bool{}
	if sets, ok := v.s.Model.(recentChangeSets); ok {
		for _, set := range sets.RecentChangeSets(v.since) {
			for _, id := range set.Entities() {
				if id.Kind == kube.KindNode && !v.s.Model.Exists(id) &&
					v.deleted(id) {
					found[id] = true
				}
			}
		}
	}
	out := sortedKeys(found)
	v.removed = &out
	return out
}

// deleted reports a deletion of id in the window.
func (v *view) deleted(id inventory.EntityID) bool {
	for _, change := range v.changesOf(id) {
		if change.Deleted {
			return true
		}
	}
	return false
}

// capacityLeft reports a scheduler capacity reason ("Insufficient cpu")
// for a pod that began waiting right after a node was removed: the
// capacity that is missing is the capacity that node had.
func (v *view) capacityLeft(reason string, pod inventory.EntityID) bool {
	return strings.HasPrefix(reason, "Insufficient") &&
		len(v.removedNodeHops(pod)) > 0
}

// drainBlockerHops lead from a draining node to the disruption budgets
// of the pods on it; the row decides whether one blocks the drain. The
// hop does not wait for the budget's own finding, so the budget is an
// input of the node's area before it fails.
func (v *view) drainBlockerHops(node inventory.EntityID) []hop {
	if !v.hasMode(node, []detection.Mode{detection.ModeDraining}) {
		return nil
	}
	var out []hop
	for _, budget := range v.s.Model.Entities(kube.KindPDB) {
		for pod := range v.selected(budget) {
			if v.runsOn(pod, node) {
				out = append(out, hop{link: LinkBlocks, to: budget})
				break
			}
		}
	}
	return out
}

// runsOn reports whether pod runs on node.
func (v *view) runsOn(pod, node inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		if id == node {
			return true
		}
	}
	return false
}

// operatorRows cover operators (ADR 0010 rule 19). A custom resource's
// own condition is its operator's verdict on it, not a summary of its
// pods as a Deployment's condition is, so a failing condition explains
// what the resource owns. A failing operator workload stopping its
// resources is the generic controller-failing row.
var operatorRows = []Row{
	{
		Name: "custom-resource-failing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind, Custom: true,
			Modes: []detection.Mode{
				detection.ModeCondition, detection.ModeNotReconciling}},
		Link: LinkOwns, Effect: anything, Prior: 0.7, Inside: true,
	},
}
