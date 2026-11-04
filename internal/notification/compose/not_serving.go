package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A flapping workload whose readiness check fails, or whose Service has
// no ready endpoint, is out of service while it is down: that is what
// its users see, so the lead says it before the flapping ("api in shop
// is not serving: its readiness check fails (connection refused) and it
// keeps restarting"). The incident stays flapping; only the words
// change.

// readinessClasses say in a few words how a readiness probe failed,
// from the kubelet's message; the first match wins.
var readinessClasses = []struct {
	pattern *regexp.Regexp
	words   string
}{
	{regexp.MustCompile(`connection refused`), "connection refused"},
	{regexp.MustCompile(`deadline exceeded|timed out|timeout`),
		"timed out"},
	{regexp.MustCompile(`statuscode: (\d{3})`), "HTTP $1"},
	{regexp.MustCompile(`connection reset`), "connection reset"},
	{regexp.MustCompile(`no route to host`), "no route to host"},
}

// restartModes are member modes that mean the workload restarts.
var restartModes = []detection.Mode{detection.ModeCrashLoop,
	detection.ModeRestarting, detection.ModeExit,
	detection.ModeProbeLiveness, detection.ModeOOMKilled,
	detection.ModeError}

// notServingState is the flapping state of a workload that is out of
// service, without its name; ok is false when nothing says users cannot
// reach it.
func notServingState(f caseFacts) (string, bool) {
	probe, readiness := readinessFailure(f.members)
	if !readiness && !serviceEmpty(f.members) {
		return "", false
	}
	state := "is not serving: its Service has no ready endpoint"
	if readiness {
		state = "is not serving: its readiness check fails"
		if words := readinessWords(probe); words != "" {
			state += " (" + words + ")"
		}
	}
	if restarts(f.members) {
		state += " and it keeps restarting"
	}
	return state, true
}

// flappingState is what a flapping subject is doing, in the lead.
func flappingState(f caseFacts) string {
	if state, ok := notServingState(f); ok {
		if !restarts(f.members) {
			state += " and it keeps failing and recovering"
		}
		return state
	}
	return "keeps failing and recovering"
}

// readinessFailure is the kubelet's message of a failing readiness
// probe among members.
func readinessFailure(members []detection.Finding) (string, bool) {
	for _, m := range failing(members) {
		if m.Reason == reasons.ReadinessProbeFailed ||
			m.Mode.Within(detection.ModeProbeReadiness) {
			return evidence(m, "probe"), true
		}
	}
	return "", false
}

// serviceEmpty reports a member Service with no ready endpoint.
func serviceEmpty(members []detection.Finding) bool {
	for _, m := range failing(members) {
		if m.Entity.Kind == kube.KindService &&
			m.Mode.Within(detection.ModeNoEndpoints) {
			return true
		}
	}
	return false
}

// restarts reports a member that restarts: a crash, an exit, a
// liveness kill.
func restarts(members []detection.Finding) bool {
	for _, m := range failing(members) {
		if m.Reason == reasons.HighRestartCount ||
			m.Reason == reasons.LivenessKilled ||
			anyWithin(m.Mode, restartModes) ||
			evidence(m, "restarts") != "" {
			return true
		}
	}
	return false
}

func anyWithin(mode detection.Mode, modes []detection.Mode) bool {
	for _, m := range modes {
		if mode.Within(m) {
			return true
		}
	}
	return false
}

// readinessWords say how a readiness probe failed, or "".
func readinessWords(message string) string {
	lower := strings.ToLower(message)
	for _, c := range readinessClasses {
		if m := c.pattern.FindStringSubmatchIndex(lower); m != nil {
			return string(c.pattern.ExpandString(nil, c.words, lower, m))
		}
	}
	return ""
}

// flappingUpdate opens the flapping update's count: "keeps failing and
// recovering: it has recovered ", or the out-of-service state first.
func flappingUpdate(f caseFacts) string {
	if state, ok := notServingState(f); ok {
		return state + "; it has recovered "
	}
	return "keeps failing and recovering: it has recovered "
}
