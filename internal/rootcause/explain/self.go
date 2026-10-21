package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkSelf joins a failure to its own workload: no other entity is in
// between. Rows with this link say why a workload is its own cause,
// such as its own spec change or a limit set too low. The walk never
// follows it; addSelf matches it.
const LinkSelf LinkType = "self"

// selfLinked reports a candidate blamed through a LinkSelf row for every
// failure it explains: the workload's own setting, whose own pods are
// the failures, not independent support for it.
func (c *candidate) selfLinked() bool {
	direct := c.direct()
	for _, effect := range direct {
		if c.covers[effect].link != LinkSelf {
			return false
		}
	}
	return len(direct) > 0
}

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
	if root != effect || !v.changedBefore(root, effect) ||
		v.onlyNotReconciling(effect) {
		// An owner's change reaches its pods through the rollout row;
		// a change after the failure began did not cause it.
		cause.modes = withoutChanges(cause.modes)
	}
	return bestRow(propagation, cause, LinkSelf, v.effectState(effect))
}

// onlyNotReconciling reports an effect whose only failure is that its
// controller has not processed its latest change: the change is what
// waits, so it cannot be what broke the object. A controller that read
// the change and reported an error is not waiting.
func (v *view) onlyNotReconciling(effect inventory.EntityID) bool {
	var waiting bool
	for _, f := range v.s.Findings[effect] {
		if f.Health == detection.Healthy || f.Advisory {
			continue
		}
		if f.Reason != reasons.GenerationLagging {
			return false
		}
		waiting = true
	}
	return waiting
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
