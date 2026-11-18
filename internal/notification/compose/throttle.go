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
		state := cpuState(m)
		if !probe || state == "" {
			continue
		}
		verb := " failed"
		if probeTimeout.MatchString(evidence(m, "probe")) {
			verb = " timed out"
		}
		text := name + verb + " while the container " + state
		if peak := evidence(m, detection.EvidenceCPUPeak); peak != "" {
			text += "; its CPU peaked at " + peak + " in the last 24 hours"
		}
		return []sentence{{part: partProof, weight: weightThrottle,
			text: endSentence(text)}}
	}
	return nil
}

// cpuState words how the container stood against its CPU limit: "was
// CPU-throttled 72% of the time (limit 200m)" or "used 248m of its 250m
// CPU limit". "" when it was not at its limit.
func cpuState(m detection.Finding) string {
	limit := evidence(m, detection.EvidenceCPULimit)
	if pct := evidence(m, detection.EvidenceCPUThrottled); pct != "" {
		text := "was CPU-throttled " + pct + " of the time"
		if limit != "" {
			text += " (limit " + limit + ")"
		}
		return text
	}
	used := evidence(m, detection.EvidenceCPUAtLimit)
	if used == "" || limit == "" {
		return ""
	}
	return "used " + used + " of its " + limit + " CPU limit"
}
