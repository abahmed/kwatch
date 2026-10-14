package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// heldVolumeRules are the explain rows that blame what still holds the
// volume a pod waits for.
var heldVolumeRules = []string{"volume-held-by-pod", "volume-held-by-node"}

// volumeHeldLead leads a multi-attach incident with the waiting
// workload and what holds its volume: "api in shop can't start: its
// volume pvc-data is still attached to node n1 (used by pod api-6d,
// terminating)". The facts are the attach error's evidence; a part the
// detector could not find is left out.
func volumeHeldLead(f caseFacts) (string, bool) {
	if !causeRuleIs(f, heldVolumeRules...) {
		return "", false
	}
	var waiting detection.Finding
	found := false
	for _, m := range f.members {
		if m.Reason == reasons.VolumeMultiAttach {
			waiting, found = m, true
			break
		}
	}
	if !found {
		return "", false
	}
	text := f.leadName(symptomSubject(f)) + " can't start: its volume"
	if volume := evidence(waiting, detection.EvidenceHeldVolume); volume != "" {
		text += " " + volume
	}
	text += " is still attached to"
	if node := evidence(waiting, detection.EvidenceAttachNode); node != "" {
		text += " node " + node
	} else {
		text += " another node"
	}
	var used []string
	if pod := evidence(waiting, detection.EvidenceHeldPod); pod != "" {
		used = append(used, "used by pod "+pod)
	}
	if evidence(waiting, detection.EvidenceHeldPodEnding) != "" {
		used = append(used, "terminating")
	}
	if len(used) > 0 {
		text += " (" + strings.Join(used, ", ") + ")"
	}
	return text, true
}
