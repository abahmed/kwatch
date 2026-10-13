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
