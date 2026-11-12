package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
)

// probeLeadKinds are the probes probeLead words, in the order it looks
// for them, with what a failing one does to the workload. A liveness
// kill comes first: being restarted is worse than being out of traffic.
var probeLeadKinds = []struct{ reason, effect string }{
	{reasons.LivenessProbeFailed, "keeps being restarted"},
	{reasons.StartupProbeFailed, "can't finish starting"},
	{reasons.ReadinessProbeFailed, "is not ready"},
}

// probeLead words a workload whose own health check fails when nothing
// else explains it, plainly, as a junior reads it: "catalog in shop is
// not ready: its readiness check (/readyz on port 8081) answers HTTP
// 503" (it is taken out of traffic), "… keeps being restarted: its
// liveness check fails (connection refused)" (Kubernetes restarts it),
// or "… does not answer within 1s" for a check that timed out. When the
// container's last run logged an error, it is added: "… after it failed
// at start-up: ConnectException: Connection refused". The application's
// own words on an error status are proof (probeLogSentences), which a
// Community message leaves out. A probe of a port nothing listens on is
// a cause of its own and keeps its own lead.
func probeLead(f caseFacts) (string, bool) {
	m, kind, ok := probeLeadMember(f)
	// A flapping incident keeps its own words (flappingState).
	if !ok || f.p.State == incident.Flapping {
		return "", false
	}
	return f.leadName(affectedWorkload(f)) + " " + kind.effect +
		": its " + probeCheckWords[kind.reason] +
		probeTarget(evidence(m, detection.EvidenceProbeCheck)) +
		" " + probeAnswer(m) + startErrorClause(f, m), true
}

// probeLeadMember is the probe failure probeLead words, and its kind.
func probeLeadMember(f caseFacts) (detection.Finding,
	struct{ reason, effect string }, bool) {
	for _, kind := range probeLeadKinds {
		for _, m := range f.members {
			// A container that exits on its own is not killed by its
			// probe: its exit leads (ownLead).
			if m.Reason == kind.reason &&
				evidence(m, detection.EvidenceExitedOnOwn) == "" {
				return m, kind, true
			}
		}
	}
	return detection.Finding{}, probeLeadKinds[0], false
}

// probeAnswer is what a probe got: "answers HTTP 503", "does not answer
// within 1s" when it timed out, "fails (connection refused)" for
// another known result, or "fails".
func probeAnswer(m detection.Finding) string {
	result := evidence(m, detection.EvidenceProbeResult)
	switch {
	case strings.HasPrefix(result, "HTTP "):
		return "answers " + result
	case result == "timed out":
		words := "does not answer in time"
		if s := evidence(m, detection.EvidenceProbeTimeout); s != "" {
			words = "does not answer within " + s + "s"
		}
		return words
	case result != "":
		return "fails (" + result + ")"
	}
	return "fails"
}

// startErrorMax bounds the start-up error a probe lead quotes.
const startErrorMax = 72

// startErrorClause is " after it failed at start-up: …", the error line
// the probed container's last run logged (detection.EvidenceError on
// any member of its container), trimmed; empty when none is known.
func startErrorClause(f caseFacts, probe detection.Finding) string {
	for _, m := range f.members {
		if m.Entity != probe.Entity {
			continue
		}
		if line := evidence(m, detection.EvidenceError); line != "" {
			return " after it failed at start-up: " +
				clipTo(unqualifiedError(line), startErrorMax)
		}
	}
	return ""
}

// quotedByProbeLead reports an error line the probe lead quotes
// already, as its start-up error.
func quotedByProbeLead(f caseFacts, line string) bool {
	m, _, ok := probeLeadMember(f)
	if !ok || startErrorClause(f, m) == "" {
		return false
	}
	lead, isLead := probeLead(f)
	return isLead && leadText(f) == lead &&
		strings.Contains(lead, clipTo(unqualifiedError(line),
			startErrorMax))
}

// qualifiedName is a dotted type name that leads an error line
// ("java.net.ConnectException:"), as runtimes print exceptions.
var qualifiedName = regexp.MustCompile(
	`^(?:[A-Za-z_][\w]*\.)+([A-Za-z_][\w]*:)`)

// unqualifiedError drops the package path of a leading type name, which
// says where the type lives, not what failed: "java.net.ConnectException:
// refused" reads "ConnectException: refused".
func unqualifiedError(line string) string {
	return qualifiedName.ReplaceAllString(line, "$1")
}

// probeTarget is " (/readyz on port 8081)" for "http /readyz :8081",
// " (port 5432)" for a TCP check, " (gRPC on port 9090)", or nothing.
func probeTarget(check string) string {
	fields := strings.Fields(check)
	if len(fields) == 0 {
		return ""
	}
	port := strings.TrimPrefix(fields[len(fields)-1], ":")
	switch {
	case fields[0] == "http" && len(fields) >= 3:
		return " (" + fields[1] + " on port " + port + ")"
	case fields[0] == "tcp" && len(fields) >= 2:
		return " (port " + port + ")"
	case fields[0] == "grpc" && len(fields) >= 2:
		return " (gRPC on port " + port + ")"
	}
	return ""
}

// answeredByProbeLead reports a lead that says the check's answer
// already: the proof then only adds the application's words.
func answeredByProbeLead(f caseFacts) bool {
	_, ok := probeLead(f)
	return ok
}
