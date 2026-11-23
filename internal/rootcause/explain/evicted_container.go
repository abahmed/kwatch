package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// evictedPodModes are the modes of a container's pod that say the
// kubelet evicted it. The container exits with code 137 when the kubelet
// stops it, and that exit is the eviction, not a crash of its own: the
// container takes the pod's mode so the node that evicted it explains
// both.
func (v *view) evictedPodModes(id inventory.EntityID) []modeHealth {
	if id.Kind != kube.KindContainer {
		return nil
	}
	pod, ok := v.podOf(id)
	if !ok {
		return nil
	}
	var out []modeHealth
	for _, f := range v.s.Findings[pod] {
		if f.Mode.Within(detection.ModeEvicted) {
			out = append(out, findingModes([]detection.Finding{f})...)
		}
	}
	return out
}
