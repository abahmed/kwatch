package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkSelf joins a failure to its own workload: no other entity is in
// between. Rows with this link say why a workload is its own cause,
// such as its own spec change or a limit set too low. The walk never
// follows it; addSelf matches it.
const LinkSelf LinkType = "self"

// selfMatch returns the LinkSelf row that explains effect by its own
// root, if one applies. Without one the root stays a bare "self"
// candidate, whose prior is under the floor.
func (v *view) selfMatch(root, effect inventory.EntityID) (rowMatch, bool) {
	if root.Kind == kube.KindNode {
		// A node's taints and conditions are written by the node
		// lifecycle controller in reply to its health: they are
		// symptoms of the node, never edits that broke it.
		return rowMatch{}, false
	}
	cause := v.causeState(root, effect, LinkSelf)
	if root != effect || !v.changedBefore(root, effect) {
		// An owner's change reaches its pods through the rollout row;
		// a change after the failure began did not cause it.
		cause.modes = withoutChanges(cause.modes)
	}
	return bestRow(propagation, cause, LinkSelf, v.effectState(effect))
}

// changedBefore reports a change of id, other than a scale, no later
// than effect began to fail.
func (v *view) changedBefore(id, effect inventory.EntityID) bool {
	first := v.earliestSince([]inventory.EntityID{effect})
	for _, change := range v.changesOf(id) {
		if changeMode(change) == ModeScaled {
			continue
		}
		if !first.ok || !change.At.After(first.t.Add(TemporalSlack)) {
			return true
		}
	}
	return false
}

// withoutChanges drops the change pseudo modes.
func withoutChanges(modes []modeHealth) []modeHealth {
	var out []modeHealth
	for _, mh := range modes {
		if !mh.mode.Within(ModeChanged) {
			out = append(out, mh)
		}
	}
	return out
}
