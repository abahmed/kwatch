package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// probeLeadKinds are the probes probeLead words, in the order it looks
// for them, with what a failing one does to the workload.
var probeLeadKinds = []struct{ reason, effect string }{
	{reasons.ReadinessProbeFailed, "is not ready"},
	{reasons.StartupProbeFailed, "can't finish starting"},
	{reasons.LivenessProbeFailed, "is restarted"},
}

// probeLead words a workload whose own health check answers but says
// no, when nothing else explains it: "catalog in shop is not ready:
// its readiness check (/readyz on port 8081) answers HTTP 503", or "…
// does not answer within 1s" for a check that timed out. The
// application's own words are proof (probeLogSentences), which a
// Community message leaves out. A check that could not
// connect (nothing listening) is not worded here: the port is.
func probeLead(f caseFacts) (string, bool) {
	for _, kind := range probeLeadKinds {
		for _, m := range f.members {
			if m.Reason != kind.reason {
				continue
			}
			answer, ok := probeAnswer(m)
			if !ok {
				continue
			}
			return f.leadName(affectedWorkload(f)) + " " + kind.effect +
				": its " + probeCheckWords[kind.reason] +
				probeTarget(evidence(m, detection.EvidenceProbeCheck)) +
				" " + answer, true
		}
	}
	return "", false
}

// probeAnswer is what a probe got: "answers HTTP 503", or "does not
// answer within 1s" when it timed out; false for any other failure.
func probeAnswer(m detection.Finding) (string, bool) {
	result := evidence(m, detection.EvidenceProbeResult)
	switch {
	case strings.HasPrefix(result, "HTTP "):
		return "answers " + result, true
	case result == "timed out":
		words := "does not answer in time"
		if s := evidence(m, detection.EvidenceProbeTimeout); s != "" {
			words = "does not answer within " + s + "s"
		}
		return words, true
	}
	return "", false
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
