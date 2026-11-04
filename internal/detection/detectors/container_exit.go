package detectors

import (
	"regexp"
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// containerStatusUnknown is the terminated reason the kubelet records,
// with exit code 137, for a container it lost track of. It is not a
// SIGKILL and keeps the generic Error reason.
const containerStatusUnknown = "ContainerStatusUnknown"

// terminatedExit names the exit of a terminated container. OOM kills are
// handled before, so 137 here is a plain SIGKILL.
func terminatedExit(code float64, reason string) (exitMeaning, bool) {
	if reason == containerStatusUnknown {
		return exitMeaning{}, false
	}
	return exitMeaningFor(code)
}

// applyCrashLoopExit names a crash loop after the exit code of its last
// termination when that code has an unambiguous cause.
func applyCrashLoopExit(s *detection.Finding, e inventory.Entity) {
	code, ok := number(e, kube.AttrLastExitCode)
	if !ok || text(e, kube.AttrLastReason) == reasons.OOMKilled ||
		flag(e, kube.AttrInit) {
		return
	}
	meaning, ok := exitMeaningFor(code)
	if !ok || !meaning.crashLoop {
		return
	}
	s.Reason = meaning.reason
	s.Summary = containerRole(e) + " " + exitSummary(meaning, e) +
		" and is restarting"
}

// unhandledException matches the line a managed runtime prints when an
// exception ends the program: .NET "Unhandled exception.", Java
// 'Exception in thread "main"', Node "Uncaught" and the like.
var unhandledException = regexp.MustCompile(`(?i)\b(unhandled|uncaught)` +
	`\b.*\b(exception|error|rejection)\b|exception in thread "|` +
	`unhandledpromiserejection`)

// exitSummary words a decoded exit. A runtime that aborts (134) or ends
// with 139 on an unhandled exception did not segfault in its own code:
// when the container's last words show such an exception, the exit is
// worded after it. Only the words change; the reason stays the code's.
func exitSummary(meaning exitMeaning, e inventory.Entity) string {
	if meaning.code != exitSIGSEGV && meaning.code != exitSIGABRT {
		return meaning.summary
	}
	for _, attr := range []string{kube.AttrMessage, kube.AttrLastMessage,
		kube.AttrLastErrorLine} {
		if unhandledException.MatchString(text(e, attr)) {
			return "crashed with an unhandled exception (exit code " +
				strconv.Itoa(int(meaning.code)) + ")"
		}
	}
	return meaning.summary
}
