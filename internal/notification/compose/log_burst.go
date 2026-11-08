package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// weightLogBurst ranks the log line above the generic proof: it is the
// only thing known about an incident Kubernetes sees as healthy.
const weightLogBurst = 0.9

// logBurst returns the root's own finding when it reports a burst of
// errors in the log of a workload that is Running and Ready.
func logBurst(
	p incident.Incident, members []detection.Finding,
) *detection.Finding {
	own := rootFinding(p, members)
	if own == nil || own.Mode != detection.ModeErrorBurst {
		return nil
	}
	return own
}

// logBurstLead states what Kubernetes cannot: "orders in shop is up but
// failing: 340 errors in 5 min".
func logBurstLead(f caseFacts) (string, bool) {
	own := logBurst(f.p, f.members)
	if own == nil {
		return "", false
	}
	return f.leadName(own.Entity) + " is up but failing: " +
		strings.TrimSuffix(own.Summary, "."), true
}

// logBurstSentences say what the platform reports and what the log
// shows, against the workload's own earlier windows: "Pods are Running
// and Ready; the log shows "pq: connection refused" 312 times, up from
// 2 in the previous hour."
func logBurstSentences(f caseFacts) []sentence {
	own := logBurst(f.p, f.members)
	if own == nil {
		return nil
	}
	text := "Pods are Running and Ready"
	line := evidence(*own, detection.EvidenceLogLine)
	if line == "" {
		return []sentence{{part: partProof, weight: weightLogBurst,
			text: text + "."}}
	}
	count := evidence(*own, detection.EvidenceLogLineCount)
	text += "; the log shows " + quoted(line) + " " + count + " times"
	return []sentence{{part: partProof, weight: weightLogBurst,
		text: text + logBaselineTail(*own) + "."}}
}

// logBaselineTail is ", up from 2 in the previous hour", or ", no
// earlier errors seen" when the earlier windows held none of the line.
func logBaselineTail(own detection.Finding) string {
	before, _ := strconv.Atoi(evidence(own, detection.EvidenceLogLineBefore))
	if before == 0 {
		return ", no earlier errors seen"
	}
	return ", up from " + strconv.Itoa(before) + " in " + previousSpan(own)
}

// previousSpan names the baseline: "the previous hour" for a full one,
// else its length, so the words never claim more history than was read.
func previousSpan(own detection.Finding) string {
	seconds, _ := strconv.Atoi(
		evidence(own, detection.EvidenceLogBaselineSpan))
	if minutes := seconds / 60; minutes < 55 {
		return "the previous " + strconv.Itoa(max(minutes, 1)) + " min"
	}
	return "the previous hour"
}

// logBurstSteps read the recent errors of the workload.
func logBurstSteps(own detection.Finding) []notification.Step {
	window, _ := strconv.Atoi(evidence(own, detection.EvidenceLogWindow))
	id := own.Entity
	return []notification.Step{{
		Text: "Read the recent errors",
		Command: "kubectl logs " + logsRef(id) + namespaceFlag(
			id.Namespace) + " --since=" + strconv.Itoa(max(window/60, 1)) +
			"m | grep -i error",
	}}
}

// logsRef is "deploy/orders" for a Deployment, else kind/name.
func logsRef(id inventory.EntityID) string {
	kind := string(id.Kind)
	if kind == "deployment" {
		kind = "deploy"
	}
	return kind + "/" + quote(id.Name)
}
