package filter

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

// DefaultNotReadyThreshold is the built-in duration a pod must stay not
// ready before an alert fires. It is intentionally not configurable — the
// detection should just work out of the box.
const DefaultNotReadyThreshold = 60 * time.Second

// NotReadyFilter alerts when a pod has been not ready (PodReady=False) for
// longer than Threshold even though all its containers are running and have
// not crashed. The container detectors intentionally skip running containers
// with no restarts, so the classic "readiness probe failing while the app is
// up" case would otherwise produce no alert at all.
type NotReadyFilter struct {
	Threshold time.Duration
}

func (f NotReadyFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil {
		return StatusContinue
	}
	if ctx.Pod.Status.Phase != corev1.PodRunning {
		return StatusContinue
	}

	// A container-level failure (crash, waiting, terminated with non-zero
	// exit) is handled by the container pipeline — don't double-alert here.
	for _, cs := range ctx.Pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			return StatusContinue
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 &&
			cs.State.Terminated.Reason != "Completed" {
			return StatusContinue
		}
	}

	// Already alerting for this pod at pod level with the same reason.
	if ctx.PodLastState != nil && ctx.PodLastState.Reason == constant.ReasonContainersNotReady {
		return StatusContinue
	}

	var lastTransition time.Time
	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			if c.Status == corev1.ConditionTrue {
				return StatusContinue
			}
			lastTransition = c.LastTransitionTime.Time
			break
		}
	}
	if lastTransition.IsZero() {
		return StatusContinue
	}

	refTime := lastTransition
	if !ctx.Config.WatchStartTime.IsZero() && ctx.Config.WatchStartTime.After(refTime) {
		refTime = ctx.Config.WatchStartTime
	}
	if time.Since(refTime) < f.Threshold {
		return StatusContinue
	}

	ctx.PodHasIssues = true
	ctx.ContainersHasIssues = false
	ctx.PodReason = constant.ReasonContainersNotReady
	ctx.PodMsg = fmt.Sprintf("pod has been not ready for %s", f.Threshold.Round(time.Second).String())

	return StatusAlert
}

func (f NotReadyFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
