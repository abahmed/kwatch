package compose

import (
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightReadinessFlap is the proof of readiness flapping: it says what the
// clients of the workload see.
const weightReadinessFlap = 0.65

// readinessFlapSentences say what readiness flapping does to the Service in
// front of the pods and quote the kubelet's readiness failure as it
// wrote it: `Service api's endpoints keep changing. Quoted readiness
// failure: "Readiness probe failed: ..."`.
func readinessFlapSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if m.Reason != reasons.ReadinessFlapping {
			continue
		}
		text := ""
		if service := evidence(m, "service"); service != "" {
			text = "Service " + service + "'s endpoints keep changing."
		}
		if line := evidence(m, "readiness probe"); line != "" {
			if text != "" {
				text += " "
			}
			text += "Quoted readiness failure: " + quoted(line) + "."
		}
		if text == "" {
			return nil
		}
		return []sentence{proof(weightReadinessFlap, text)}
	}
	return nil
}
