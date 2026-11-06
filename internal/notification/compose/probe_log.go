package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// weightProbeLog ranks the application's own words on why its probe
// gets an error status: it is the strongest fact such a note has.
const weightProbeLog = 0.65

// probeCheckWords name a probe finding's check.
var probeCheckWords = map[string]string{
	reasons.ReadinessProbeFailed: "readiness check",
	reasons.LivenessProbeFailed:  "liveness check",
	reasons.StartupProbeFailed:   "startup check",
}

// probeLogSentences put the line the application logged next to the
// error status its probe got: "Its readiness check answers HTTP 500
// (from its log: "…")." The line is quoted, never interpreted. It is
// proof, which a Community message leaves out.
func probeLogSentences(f caseFacts) []sentence {
	answered := answeredByProbeLead(f)
	for _, m := range f.members {
		line := evidence(m, detection.EvidenceProbeLog)
		result := evidence(m, detection.EvidenceProbeResult)
		check, ok := probeCheckWords[m.Reason]
		if line == "" || !ok || !strings.HasPrefix(result, "HTTP ") {
			continue
		}
		subject := "Its " + check
		if !f.aboutLead(m.Entity) {
			subject = "The " + check + " of " + f.memberName(m.Entity)
		}
		text := subject + " answers " + result + " (from its log: " +
			quoted(line) + ")."
		if answered {
			text = "From its log: " + quoted(line) + "."
		}
		return []sentence{{part: partProof, weight: weightProbeLog,
			text: text}}
	}
	return nil
}
