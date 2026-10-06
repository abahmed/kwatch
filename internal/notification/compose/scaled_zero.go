package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightScaledZero ranks who scaled a workload to zero: it is the whole
// proof of a scale-down that is not an outage of the workload itself.
const weightScaledZero = 0.7

// scaledZeroSentences say who scaled a routed workload to zero and
// when, as far as the change history knows: "It was scaled to 0 by
// kubectl-scale at 21:10." The actor is the field manager's name, shown
// as recorded.
func scaledZeroSentences(f caseFacts) []sentence {
	for _, m := range f.members {
		if m.Reason != reasons.ScaledToZeroRouted {
			continue
		}
		by := evidence(m, detection.EvidenceScaledBy)
		at, err := time.Parse(time.RFC3339,
			evidence(m, detection.EvidenceScaledAt))
		text := "It was scaled to 0"
		if by != "" {
			text += " by " + by
		}
		if err == nil {
			text += " at " + clock(at)
		}
		if by == "" && err != nil {
			return nil
		}
		return []sentence{{part: partProof, weight: weightScaledZero,
			text: text + "."}}
	}
	return nil
}
