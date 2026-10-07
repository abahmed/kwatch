package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightLivenessStart ranks the start-time facts with the other direct
// proofs of a kill: they are the numbers behind the blamed probe.
const weightLivenessStart = 0.7

// livenessStartRules are the rows that blame a liveness probe that
// kills a container before it has started.
var livenessStartRules = map[string]bool{
	"liveness-shorter-than-start": true,
	"liveness-kills-before-ready": true,
}

// livenessStartSentences say what liveness allows and what a start
// needs: "Liveness gives api 40s (10s delay + 3 × 10s) but api
// normally needs about 75s to become ready (last 5 starts), so it is
// killed before it finishes starting." Without history of starts they
// say only that the container was killed before it was ready. They say
// nothing unless the cause is the liveness budget.
func livenessStartSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || !livenessStartRules[f.p.Cause.Rule] {
		return nil
	}
	name := shortName(f.p.Cause.Root)
	for _, m := range f.members {
		if m.Reason != reasons.LivenessKilled {
			continue
		}
		gives := evidence(m, detection.EvidenceLivenessGives)
		if gives == "" {
			continue
		}
		return []sentence{{part: partProof, weight: weightLivenessStart,
			text: livenessStartText(m, name, gives)}}
	}
	return nil
}

func livenessStartText(m detection.Finding, name, gives string) string {
	usual := evidence(m, detection.EvidenceUsualStart)
	if usual == "" {
		return "Liveness gives " + name + " " + gives + ", and " + name +
			" was not ready when each kill came."
	}
	text := "Liveness gives " + name + " " + gives + " but " + name +
		" normally needs about " + usual + " to become ready"
	if n := evidence(m, detection.EvidenceStartSamples); n != "" {
		text += " (last " + n + " starts)"
	}
	return text + ", so it is killed before it finishes starting. " +
		"A startupProbe or a longer delay would let it start."
}
