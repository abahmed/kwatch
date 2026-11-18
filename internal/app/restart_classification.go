package app

import (
	"fmt"
	"time"
)

// classifyRestart explains how the previous session ended, compared
// with this process (envIdentity). "" is a first run or a normal
// restart.
func classifyRestart(previous runtimeSession) string {
	return classifyRestartAt(previous, envIdentity(""))
}

// classifyRestartAt is classifyRestart against a given identity of
// this start.
func classifyRestartAt(previous runtimeSession, now runIdentity) string {
	if previous.SessionID == "" {
		return ""
	}
	if previous.EndReason == "graceful_shutdown" {
		// Asked to stop: a rollout, a drained or lost node, an
		// eviction, or a plain restart (classifyStop).
		return classifyStop(previous, now)
	}
	if previous.EndReason != "" {
		return normalizeRestartReason(previous.EndReason)
	}
	if !previous.EndedAt.IsZero() {
		return ""
	}
	if previous.NodeName != "" && previous.NodeName != now.Node {
		return "node_disruption"
	}
	if previous.PodName != "" && previous.PodName != now.Pod {
		return "deployment_rollout"
	}
	return "internal_failure"
}

// wantsEvidence reports whether the API is asked about the previous
// pod and node: after a crash, and after a stop on a signal when the
// pod was replaced (an evicted pod stays behind with reason Evicted).
func wantsEvidence(previous runtimeSession, now runIdentity) bool {
	if previous.SessionID == "" {
		return false
	}
	if previous.EndedAt.IsZero() {
		return true
	}
	return previous.EndReason == "graceful_shutdown" &&
		changed(previous.PodName, now.Pod)
}

func classifyWithEvidence(
	previous runtimeSession,
	evidence restartEvidence,
	fallback string,
) string {
	if previous.EndReason == "graceful_shutdown" {
		return stopEvidence(evidence, fallback)
	}
	if previous.EndReason != "" {
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
	if nodeGone(evidence) {
		return "node_disruption"
	}
	return fallback
}

// stopEvidence adds what the API shows to a stop on a signal that the
// stop facts did not explain: the old pod evicted, or its node gone or
// not ready.
func stopEvidence(evidence restartEvidence, fallback string) string {
	switch {
	case fallback != "" && fallback != "node_disruption":
		return fallback
	case evidence.PodReason == "Evicted":
		return "eviction"
	case nodeGone(evidence):
		return "node_disruption"
	}
	return fallback
}

func nodeGone(evidence restartEvidence) bool {
	return !evidence.NodeReady &&
		(evidence.NodeObserved || evidence.NodeReason != "")
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
