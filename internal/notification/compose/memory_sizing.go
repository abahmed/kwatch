package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightSizing ranks the observed numbers behind a kill by the
// container's own limit below the memory history itself: it is what a
// reader needs to choose a value, not the proof of the kill.
const weightSizing = 0.5

// factSizing is the peak against the limit, said once.
const factSizing factKey = "sizing"

// fullWindow is the window when kwatch has the whole day of history.
const fullWindow = "24h"

// sizingSentences state, for a container killed at its own memory
// limit, the observed peak against its limit and request over the
// history kwatch really has: "In the last 5h, all the history kwatch
// has, its memory peaked at 510Mi (limit 512Mi, request 128Mi); it was
// near the limit in 4 of those hours, so how much more it needs is not
// known." They are observations at the time of a failure, never a
// recommendation, and say nothing without the history.
func sizingSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if m.Reason != reasons.OOMKilled ||
			evidence(m, detection.EvidenceOOMKind) !=
				"its own memory limit" {
			continue
		}
		if text := sizingText(m); text != "" {
			return []sentence{{part: partProof, weight: weightSizing,
				text: text, fact: factSizing}}
		}
	}
	return nil
}

// sizingText words the sizing facts of one kill, or "" without a peak
// and a limit.
func sizingText(m detection.Finding) string {
	peak := evidence(m, detection.EvidenceMemoryPeak)
	limit := evidence(m, detection.EvidenceMemoryLimit)
	window := evidence(m, detection.EvidenceMemoryWindow)
	if peak == "" || limit == "" || window == "" {
		return ""
	}
	text := "In the last " + window
	if window != fullWindow {
		text += ", all the history kwatch has,"
	}
	text += " its memory peaked at " + peak + " (limit " + limit
	if request := evidence(m, detection.EvidenceMemoryRequest); request !=
		"" {
		text += ", request " + request
	}
	text += ")"
	switch hours := evidence(m, detection.EvidenceMemoryNearLimit); {
	case hours != "":
		text += "; it was near the limit in " + hours + " of those " +
			"hours, so how much more it needs is not known"
	case usageClipped(m):
		text += "; that is near the limit, so how much more it needs " +
			"is not known"
	}
	return endSentence(text)
}
