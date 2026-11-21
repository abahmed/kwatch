package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scoreOwnTemplate vetoes a scheduling constraint as the cause of new
// pods whose own release explains why they cannot be placed: a request
// larger than any node, or a spread the template requires, set by a
// workload that changed in the window. The release is the cause to act
// on; the scheduler only reports the effect.
func scoreOwnTemplate(v *view, c *candidate) outcome {
	if c.id.Kind != KindScheduling || len(c.others()) == 0 {
		return outcome{}
	}
	for _, pod := range c.others() {
		if !v.ownerChanged(pod) || !v.templateUnplaceable(pod) {
			return outcome{}
		}
	}
	return outcome{veto: "the release changed what its pods ask for, " +
		"and no node can take that"}
}

// templateUnplaceable reports a pod whose own template cannot be placed
// whatever the cluster holds.
func (v *view) templateUnplaceable(pod inventory.EntityID) bool {
	for _, f := range v.s.Findings[pod] {
		for _, e := range f.Evidence {
			if e.Label == detectors.LabelExceedsNodes {
				return true
			}
			if hasSignal(SignalSpread, e.Value) {
				return true
			}
		}
	}
	for _, note := range v.s.Model.Notes(pod, v.since) {
		if strings.Contains(strings.ToLower(note.Message),
			"pod anti-affinity rules") {
			return pod.Kind == kube.KindPod
		}
	}
	return false
}

func init() {
	scorers = append(scorers, scorer{"own-template", scoreOwnTemplate})
}
