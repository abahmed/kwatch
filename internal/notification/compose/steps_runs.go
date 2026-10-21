package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/notification"
)

// restartingReasons are container findings that only exist after the
// container was restarted, so a previous run is there to read.
var restartingReasons = map[string]bool{
	reasons.CrashLoopBackOff:    true,
	reasons.HighRestartCount:    true,
	reasons.LivenessKilled:      true,
	reasons.LivenessProbeFailed: true,
	reasons.StartupProbeFailed:  true,
}

// ranBefore reports a container with a previous run to read: its
// finding carries a restart count or the last termination, or is one
// that follows a restart. A container that never restarted (not ready,
// throttled, or exited once) has none: "kubectl logs --previous" would
// fail with "previous terminated container not found".
func ranBefore(s detection.Finding) bool {
	if restartingReasons[s.Reason] {
		return true
	}
	return evidence(s, "restarts", "last termination",
		"last exit code") != ""
}

// currentRunSteps read a container that never restarted: the pod's
// state and events, then what the container prints now.
func currentRunSteps(pod, container, ns string) []notification.Step {
	return []notification.Step{
		{Text: "See the pod's state and events",
			Command: "kubectl describe pod " + quote(pod) + ns},
		{Text: "Read what the container prints",
			Command: "kubectl logs " + quote(pod) + " -c " +
				quote(container) + ns},
	}
}
