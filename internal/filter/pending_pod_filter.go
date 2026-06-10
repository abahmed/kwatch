package filter

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type PendingPodFilter struct {
	Threshold time.Duration
}

func (f PendingPodFilter) Detect(ctx *Context) Status {
	if ctx.Pod.Status.Phase != corev1.PodPending {
		return StatusSkip
	}

	if time.Since(ctx.Pod.CreationTimestamp.Time) < f.Threshold {
		return StatusSkip
	}

	if ctx.PodLastState != nil && ctx.PodLastState.Reason == ctx.PodReason {
		return StatusSkip
	}

	ctx.PodHasIssues = true
	ctx.ContainersHasIssues = false

	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			ctx.PodReason = c.Reason
			ctx.PodMsg = c.Message
			return StatusAlert
		}
	}

	if ctx.PodReason == "" {
		ctx.PodReason = "PodPending"
		ctx.PodMsg = "pod has been in Pending phase for " + f.Threshold.Round(time.Second).String()
	}

	return StatusAlert
}

func (f PendingPodFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
