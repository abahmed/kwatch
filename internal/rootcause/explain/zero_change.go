package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// zeroNotFromChange reports a workload whose only failure is that it is
// held at zero replicas while none of its spec changes set the replica
// count: a new image or setting synced into a workload at zero did not
// take its replicas away. A change of only the replica count is a
// scale, which own-change never blames.
func (v *view) zeroNotFromChange(root, effect inventory.EntityID) bool {
	if !v.onlyScaledToZero(effect) {
		return false
	}
	for _, change := range v.changesOf(root) {
		if changeMode(change) == ModeScaled {
			continue
		}
		for _, field := range change.Fields {
			if field.Path == replicasField {
				return false
			}
		}
	}
	return true
}

// onlyScaledToZero reports an effect whose every unhealthy finding says
// it is held at zero replicas.
func (v *view) onlyScaledToZero(effect inventory.EntityID) bool {
	found := false
	for _, f := range v.s.Findings[effect] {
		if !unhealthy(f) {
			continue
		}
		if !f.Mode.Within(detection.ModeScaledToZero) {
			return false
		}
		found = true
	}
	return found
}
