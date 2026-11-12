package compose

import (
	"regexp"
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

// weightProbeFailure ranks the kubelet's own words on a failing probe:
// below the application's words, which say why.
const weightProbeFailure = 0.6

// probeFailureSentences quote the kubelet's words on the probe a probe
// lead names: "Kubernetes reports: "dial tcp 10.0.0.7:8080: connect:
// connection refused" (3 failures)." It is proof, only when the probe
// is the lead (no deeper cause) and says no error status, which the
// lead words already.
func probeFailureSentences(f caseFacts) []sentence {
	m, _, ok := probeLeadMember(f)
	// An error status is said by the lead already ("answers HTTP 500").
	if !ok || evidence(m, detection.EvidenceProbeLog) != "" ||
		strings.HasPrefix(evidence(m, detection.EvidenceProbeResult),
			"HTTP ") {
		return nil
	}
	text := probeFailureWords(evidence(m, "probe"))
	if lead, isLead := probeLead(f); text == "" || !isLead ||
		leadText(f) != lead {
		return nil
	}
	if n := evidence(m, "failures"); n != "" {
		text += " (" + n + " failures)"
	}
	return []sentence{{part: partProof, weight: weightProbeFailure,
		text: "Kubernetes reports: " + text + "."}}
}

// probeURL is the request part of an HTTP probe message: "Liveness probe
// failed: Get "http://10.0.0.7:8080/live": " before the error.
var probeURL = regexp.MustCompile(`^.*? probe (?:failed|errored)[^:]*: ` +
	`[A-Za-z]+ "[^"]*": `)

// probeFailureWords is the error a probe failed with, quoted: the
// kubelet's message without the URL it called, which the lead already
// names by path and port and which a message never carries.
func probeFailureWords(message string) string {
	if message == "" {
		return ""
	}
	if rest := probeURL.ReplaceAllString(message, ""); rest != message {
		return quoted(rest)
	}
	if strings.Contains(message, "://") {
		return ""
	}
	return quoted(message)
}
