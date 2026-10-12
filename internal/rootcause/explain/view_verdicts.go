package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// containerVerdicts are the OOM verdicts that say a pod's containers
// were killed at their own limit.
func (v *view) containerVerdicts(pod inventory.EntityID) []detection.Mode {
	if pod.Kind != kube.KindPod {
		return nil
	}
	var out []detection.Mode
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		for _, f := range v.s.Findings[id] {
			if !f.Advisory && f.Mode.Within(detection.ModeOOMKilledLimit) {
				out = append(out, f.Mode)
			}
		}
	}
	return out
}
