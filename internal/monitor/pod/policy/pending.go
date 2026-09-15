package policy

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type PendingPodRule struct {
	Threshold time.Duration
}

func (rule PendingPodRule) Detect(ctx *Context) Decision {
	if ctx.Pod == nil {
		return DecisionDefer
	}
	if ctx.Pod.Status.Phase != corev1.PodPending {
		return DecisionDefer
	}

	refTime := ctx.Pod.CreationTimestamp.Time
	if refTime.IsZero() {
		refTime = ctx.now()
	}
	watchStart := ctx.watchStartTime()
	if !watchStart.IsZero() && watchStart.After(refTime) {
		refTime = watchStart
	}
	if ctx.now().Sub(refTime) < rule.Threshold {
		return DecisionDefer
	}

	if ctx.PodLastState != nil && ctx.PodLastState.Reason == ctx.PodReason {
		return DecisionDefer
	}

	ctx.PodHasIssues = true
	ctx.ContainersHasIssues = false

	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			ctx.PodReason = c.Reason
			ctx.PodMsg = c.Message
			return DecisionAlert
		}
	}

	if ctx.PodReason == "" {
		ctx.PodReason = "PodPending"
		ctx.PodMsg = "pod has been in Pending phase for " + rule.Threshold.Round(
			time.Second,
		).String()
	}

	return DecisionAlert
}
