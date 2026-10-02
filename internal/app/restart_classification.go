package app

import (
	"fmt"
	"os"
	"time"
)

func classifyRestart(previous runtimeSession) string {
	if previous.SessionID == "" {
		return ""
	}
	if previous.FailureCode != "" {
		return normalizeRestartReason(previous.FailureCode)
	}
	if previous.EndReason == "graceful_shutdown" {
		// A clean stop, as in a rollout: nothing was interrupted.
		return ""
	}
	if previous.EndReason != "" {
		return normalizeRestartReason(previous.EndReason)
	}
	if !previous.EndedAt.IsZero() {
		return ""
	}
	if previous.NodeName != "" && previous.NodeName != os.Getenv("NODE_NAME") {
		return "node_disruption"
	}
	if previous.PodName != "" && previous.PodName != os.Getenv("POD_NAME") {
		return "deployment_rollout"
	}
	return "internal_failure"
}

func classifyWithEvidence(
	previous runtimeSession,
	evidence restartEvidence,
	fallback string,
) string {
	if previous.FailureCode != "" || previous.EndReason != "" {
		return fallback
	}
	if evidence.APIUnavailable {
		return "api_unavailable"
	}
	if evidence.ContainerReason == "OOMKilled" ||
		evidence.PodReason == "OOMKilled" {
		return "oom_killed"
	}
	if evidence.PodReason == "Evicted" {
		return "eviction"
	}
	if evidence.LeaseLost {
		return "leader_handoff"
	}
	if !evidence.NodeReady &&
		(evidence.NodeObserved || evidence.NodeReason != "") {
		return "node_disruption"
	}
	return fallback
}

func startupClaimKey(
	version string,
	firstRun, upgrade bool,
	downtime time.Duration,
	restartReason string,
) string {
	return fmt.Sprintf(
		"%s|first=%t|upgrade=%t|downtime=%d|reason=%s",
		version, firstRun, upgrade,
		downtime.Round(time.Minute).Nanoseconds(), restartReason,
	)
}

func normalizeRestartReason(reason string) string {
	switch reason {
	case "graceful_shutdown", "deployment_rollout", "leader_handoff",
		"node_disruption", "eviction", "oom_killed", "internal_failure",
		"api_unavailable", "unknown":
		return reason
	default:
		return "unknown"
	}
}
