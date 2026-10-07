package compose

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightThrottle ranks CPU starvation behind a failing probe as direct
// proof: a handler that gets no CPU answers late.
const weightThrottle = 0.68

// probeTimeout matches the kubelet's words for a probe that got no
// answer in time.
var probeTimeout = regexp.MustCompile(
	`(?i)deadline exceeded|timed? ?out|timeout`)

// probeNames are the probe kinds a throttled container can fail.
var probeNames = map[string]string{
	reasons.LivenessProbeFailed:  "Liveness",
	reasons.LivenessKilled:       "Liveness",
	reasons.ReadinessProbeFailed: "Readiness",
	reasons.StartupProbeFailed:   "Startup",
}

// throttleSentences say that a failing probe coincided with CPU
// throttling: "Liveness timed out while the container was CPU-throttled
// 72% of the time (limit 200m)." They say nothing without the reading.
func throttleSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		name, probe := probeNames[m.Reason]
		pct := evidence(m, detection.EvidenceCPUThrottled)
		if !probe || pct == "" {
			continue
		}
		verb := " failed"
		if probeTimeout.MatchString(evidence(m, "probe")) {
			verb = " timed out"
		}
		text := name + verb + " while the container was CPU-throttled " +
			pct + " of the time"
		if limit := evidence(m, detection.EvidenceCPULimit); limit != "" {
			text += " (limit " + limit + ")"
		}
		return []sentence{{part: partProof, weight: weightThrottle,
			text: endSentence(text)}}
	}
	return nil
}
