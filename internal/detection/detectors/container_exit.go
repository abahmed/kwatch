package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// containerStatusUnknown is the terminated reason the kubelet records,
// with exit code 137, for a container it lost track of. It is not a
// SIGKILL and keeps the generic Error reason.
const containerStatusUnknown = "ContainerStatusUnknown"

// exitMeaning names a container exit code with a well-known cause.
type exitMeaning struct {
	code    float64
	reason  string
	summary string
	// crashLoop reports whether a crash loop with this last exit code is
	// named after it. SIGKILL is not: liveness probe kills end with 137,
	// and the probe finding names that cause.
	crashLoop bool
}

// exitMeanings are the shell and signal exit codes whose cause is
// unambiguous. 143 (SIGTERM) is not listed: it is how a container ends
// when Kubernetes stops it, handled by stoppedOnPurpose.
var exitMeanings = []exitMeaning{
	{126, reasons.ExitNotExecutable,
		"cannot run its command: the file is not executable (exit " +
			"code 126)", true},
	{127, reasons.ExitCommandNotFound,
		"cannot run its command: the command was not found (exit " +
			"code 127)", true},
	{exitSIGKILL, reasons.ExitKilled,
		"was killed by SIGKILL (exit code 137) without running out " +
			"of memory", false},
	{139, reasons.ExitSegfault,
		"crashed with a segmentation fault (exit code 139)", true},
}

func exitMeaningFor(code float64) (exitMeaning, bool) {
	for _, meaning := range exitMeanings {
		if meaning.code == code {
			return meaning, true
		}
	}
	return exitMeaning{}, false
}

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
	s.Summary = containerRole(e) + " " + meaning.summary +
		" and is restarting"
}
