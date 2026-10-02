package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// In-place resize conditions and their reasons (core/v1 types.go).
const (
	podResizePending    = "PodResizePending"
	podResizeInProgress = "PodResizeInProgress"
	resizeInfeasible    = "Infeasible"
	resizeDeferred      = "Deferred"
	resizeError         = "Error"
	// resizeDeferredGrace is how long a deferred resize may wait for
	// room on the node before it is reported; the kubelet retries it.
	resizeDeferredGrace = 5 * time.Minute
)

// resizeFindings reports an in-place resize the node cannot perform
// (Infeasible), keeps postponing (Deferred), or failed to apply (Error).
func resizeFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	status, reason, since := condition(e, podResizePending)
	if status == "True" {
		switch reason {
		case resizeInfeasible:
			out = append(out, resizeFinding(e, podResizePending,
				reasons.PodResizeInfeasible, since,
				"Pod resize cannot be done on its node"))
		case resizeDeferred:
			if sustained(ctx, "resize-deferred", since, resizeDeferredGrace) {
				out = append(out, resizeFinding(e, podResizePending,
					reasons.PodResizeDeferred, since,
					"Pod resize has been deferred for lack of room on its "+
						"node"))
			}
		}
	}
	status, reason, since = condition(e, podResizeInProgress)
	if status == "True" && reason == resizeError {
		out = append(out, resizeFinding(e, podResizeInProgress,
			reasons.PodResizeError, since, "Pod resize failed to apply"))
	}
	return out
}

func resizeFinding(
	e inventory.Entity, conditionType, reason string, since time.Time,
	summary string,
) detection.Finding {
	s := detection.Finding{
		Reason: reason, Severity: detection.Warning, Since: since,
		Summary: summary,
	}
	if message := conditionMessage(e, conditionType); message != "" {
		s.Evidence = []detection.Evidence{{Label: "message", Value: message}}
	}
	return s
}
