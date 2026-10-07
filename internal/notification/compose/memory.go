package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/notification"
)

// weightMemory ranks what a killed container used: it is the direct
// proof of an OOM kill, below a blamed change.
const weightMemory = 0.65

// memoryAdvice is the step that goes with a kill whose use is known.
const memoryAdvice = "Raise the memory limit above the observed peak, " +
	"or find what uses the memory"

// memorySentences say what an OOM-killed container used, from the
// memory history its finding carries. They describe the past only, and
// say nothing when the finding has no such history.
func memorySentences(f caseFacts) []sentence {
	if _, _, byNode := nodeKill(f.members); byNode {
		return nil
	}
	for _, m := range f.members {
		if m.Reason != reasons.OOMKilled {
			continue
		}
		if text := memoryText(m); text != "" {
			return []sentence{{part: partProof, weight: weightMemory,
				text: text}}
		}
	}
	return nil
}

// memoryText picks the most telling fact: a kill right after starting,
// else a steady climb, else the peak against the limit.
func memoryText(m detection.Finding) string {
	limit := evidence(m, detection.EvidenceMemoryLimit)
	if quick := evidence(m, detection.EvidenceKilledAfter); quick != "" {
		if limit == "" {
			return "It was killed within " + quick + " of starting."
		}
		return "It hit its " + limit + " memory limit within " + quick +
			" of starting."
	}
	if rise := evidence(m, detection.EvidenceMemoryRise); rise != "" {
		return "Its memory rose steadily from " + humanizeText(rise) +
			" before the kill."
	}
	peak := evidence(m, detection.EvidenceMemoryPeak)
	switch {
	case peak == "":
		return ""
	case limit == "":
		return "It used " + peak + " at peak in the last 24 hours."
	}
	return "It was killed at its " + limit + " memory limit; it used " +
		peak + " at peak in the last 24 hours."
}

// memorySteps is the advice for a kill whose memory use is known.
func memorySteps(s detection.Finding) []notification.Step {
	if s.Reason != reasons.OOMKilled ||
		evidence(s, detection.EvidenceKilledByNode) != "" ||
		(evidence(s, detection.EvidenceMemoryPeak) == "" &&
			evidence(s, detection.EvidenceKilledAfter) == "") {
		return nil
	}
	return []notification.Step{{Text: memoryAdvice}}
}
