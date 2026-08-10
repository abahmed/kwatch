package filter

import (
	"strings"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type PodStatusFilter struct{}

func (f PodStatusFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil {
		return StatusAlert
	}
	if ctx.Pod.Status.Phase == corev1.PodSucceeded {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return StatusSkip
	}

	if strings.EqualFold(ctx.EvType, "Added") && len(ctx.Pod.Status.Conditions) == 0 {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return StatusSkip
	}

	issueInContainers := true
	issueInPod := true
	for _, c := range ctx.Pod.Status.Conditions {
		switch c.Type {
		case corev1.PodReady:
			if c.Status == corev1.ConditionFalse && c.Reason == "PodCompleted" {
				ctx.PodHasIssues = false
				ctx.ContainersHasIssues = false
				return StatusSkip
			}

			issueInPod = false
			issueInContainers = false
			if c.Status != corev1.ConditionTrue {
				issueInContainers = true
			}
		case corev1.PodScheduled:
			if c.Status == corev1.ConditionFalse {
				issueInPod = true
				issueInContainers = false
				ctx.PodReason = c.Reason
				ctx.PodMsg = c.Message
			}
		case corev1.ContainersReady:
			if c.Status == corev1.ConditionFalse {
				issueInContainers = true
				issueInPod = false
			}
		case corev1.PodReadyToStartContainers:
			if c.Status == corev1.ConditionFalse {
				issueInPod = true
				issueInContainers = false
				ctx.PodReason = c.Reason
				ctx.PodMsg = c.Message
			}
		}
	}

	ctx.PodHasIssues = issueInPod
	ctx.ContainersHasIssues = issueInContainers

	if len(ctx.PodReason) > 0 &&
		len(ctx.Config.AllowedReasons) > 0 &&
		!slices.Contains(ctx.Config.AllowedReasons, ctx.PodReason) {
		klog.InfoS(
			"skipping reason for pod as it is not in the reason allow list",
			"reason", ctx.PodReason,
			"pod", ctx.Pod.Name)
		return StatusSkip
	}

	if len(ctx.PodReason) > 0 &&
		len(ctx.Config.ForbiddenReasons) > 0 &&
		slices.Contains(ctx.Config.ForbiddenReasons, ctx.PodReason) {
		klog.InfoS(
			"skipping reason for pod as it is in the reason forbid list",
			"reason", ctx.PodReason,
			"pod", ctx.Pod.Name)
		return StatusSkip
	}

	if ctx.PodHasIssues && ctx.PodLastState != nil {
		return StatusSkip
	}

	return StatusAlert
}

func (f PodStatusFilter) Execute(ctx *Context) bool {
	return f.Detect(ctx) == StatusSkip
}
