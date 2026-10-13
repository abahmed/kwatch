package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// weightHowFailed ranks how a restarting container ended. It sits below
// every other proof, which says more about why, so it fills a free
// slot of the proof and never displaces one.
const weightHowFailed = 0.05

// howFailedSentences say how long the last run lasted before it ended
// and how fast the restarts came, from numbers the detector attached.
// They describe the past only.
func howFailedSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if text := howFailedText(m); text != "" {
			return []sentence{{part: partProof, weight: weightHowFailed,
				text: text}}
		}
	}
	return nil
}

// howFailedText writes "Its last run lasted 4s; 14 restarts in
// the 2h10m since the pod was created." from whichever facts exist.
func howFailedText(m detection.Finding) string {
	ran := evidence(m, detection.EvidenceRanFor)
	span := evidence(m, detection.EvidenceRestartSpan)
	restarts := evidence(m, "restarts")
	var parts []string
	if ran != "" {
		parts = append(parts, "its last run lasted "+ran)
	}
	if span != "" && restarts != "" {
		parts = append(parts, restarts+" restarts in the "+span+
			" since its pod was created")
	}
	if len(parts) == 0 {
		return ""
	}
	text := strings.Join(parts, "; ")
	return strings.ToUpper(text[:1]) + text[1:] + "."
}
