package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// HiddenCauseMaxConfidence caps a cause that blames no concrete fault
// while a Secret or ConfigMap the failing pods read is unreadable to
// kwatch. The hidden object may be the cause, so nothing is stated: the
// message says the cause is unknown and names what is unreadable.
const HiddenCauseMaxConfidence = ConfidenceFloor - 0.01

// hiddenBlindRows are the rows that blame a workload, a helper container
// or a registry for no more than the failure itself: they hold whether
// or not a configuration object caused it.
var hiddenBlindRows = map[string]bool{
	selfRow.Name:                  true,
	"helper-container-blocks-pod": true,
	"registry-refuses":            true,
}

// hiddenConfig lists the unreadable configuration objects effect
// reads, in plain words.
func (v *view) hiddenConfig(effect inventory.EntityID) []string {
	var out []string
	for _, id := range v.inputs[effect] {
		if (id.Kind == kube.KindSecret || id.Kind == kube.KindConfigMap) &&
			!v.s.verifiable(id.Kind) {
			out = append(out, rootcause.UnverifiedScope(id))
		}
	}
	return rootcause.MergeUnverified(out)
}

// blindToHiddenCause reports a candidate that covers failures reading
// an unreadable configuration object, whose every cover is generic and
// which has no discriminating proof. It records what is unreadable.
func (v *view) blindToHiddenCause(c *candidate, got []Contribution) bool {
	// A registry named by the errors is no proof against a missing
	// login: both read the same.
	for _, g := range got {
		if g.Weight > 0 && discriminating[g.Code] &&
			c.id.Kind != kube.KindRegistry {
			return false
		}
	}
	var notes []string
	for _, effect := range c.direct() {
		how := c.covers[effect]
		if how.derived {
			continue
		}
		if !genericCover(how) {
			return false
		}
		notes = append(notes, v.hiddenConfig(effect)...)
	}
	if len(notes) == 0 {
		return false
	}
	for _, effect := range c.direct() {
		v.unverified[effect] = rootcause.MergeUnverified(
			v.unverified[effect], notes)
	}
	return true
}

// genericCover reports a cover that blames no concrete fault: the
// failure's own owner by default, or an edit or creation.
func genericCover(how coverage) bool {
	if hiddenBlindRows[how.match.row.Name] {
		return true
	}
	mode := how.match.causeMode
	return mode.Within(ModeSpecChanged) || mode.Within(ModeCreated) ||
		mode.Within(ModeChanged)
}
