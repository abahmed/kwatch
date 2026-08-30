package filter

import (
	"strings"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
)

type PodStatusFilter struct{}

// skipNonIssuePod reports pods that can never have issues: succeeded jobs and
// bare Added events without conditions.
func skipNonIssuePod(ctx *Context) (Status, bool) {
	if ctx.Pod.Status.Phase == corev1.PodSucceeded ||
		(strings.EqualFold(ctx.EvType, "Added") && len(ctx.Pod.Status.Conditions) == 0) {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return StatusSkip, true
	}
	return StatusAlert, false
}

// collectPodIssues derives issue flags from pod conditions. completed marks a
// terminated pod (Ready=False/PodCompleted) that must be skipped outright.
func collectPodIssues(ctx *Context) (issueInPod, issueInContainers, completed bool) {
	issueInPod, issueInContainers = true, true
	switch ctx.Pod.Status.Phase {
	case corev1.PodFailed:
		issueInContainers = false
		ctx.PodReason = ctx.Pod.Status.Reason
		if ctx.PodReason == "" {
			ctx.PodReason = constant.ReasonPodFailed
		}
		ctx.PodMsg = ctx.Pod.Status.Message
	case corev1.PodUnknown:
		issueInContainers = false
		ctx.PodReason = constant.ReasonPodStatusUnknown
		ctx.PodMsg = ctx.Pod.Status.Message
	}
	for _, c := range ctx.Pod.Status.Conditions {
		switch c.Type {
		case corev1.PodReady:
			if c.Status == corev1.ConditionFalse && c.Reason == "PodCompleted" {
				return false, false, true
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
	return issueInPod, issueInContainers, false
}

// skipByReasonConfig applies the reason allow/forbid lists.
func skipByReasonConfig(ctx *Context) bool {
	if len(ctx.PodReason) == 0 {
		return false
	}
	if len(ctx.Config.AllowedReasons) > 0 &&
		!slices.Contains(ctx.Config.AllowedReasons, ctx.PodReason) {
		klog.InfoS(
			"skipping reason for pod as it is not in the reason allow list",
			"reason", ctx.PodReason,
			"pod", ctx.Pod.Name)
		return true
	}
	if len(ctx.Config.ForbiddenReasons) > 0 &&
		slices.Contains(ctx.Config.ForbiddenReasons, ctx.PodReason) {
		klog.InfoS(
			"skipping reason for pod as it is in the reason forbid list",
			"reason", ctx.PodReason,
			"pod", ctx.Pod.Name)
		return true
	}
	return false
}

func (f PodStatusFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil {
		return StatusAlert
	}
	if status, skip := skipNonIssuePod(ctx); skip {
		return status
	}

	issueInPod, issueInContainers, completed := collectPodIssues(ctx)
	if completed {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return StatusSkip
	}
	ctx.PodHasIssues = issueInPod
	ctx.ContainersHasIssues = issueInContainers

	if skipByReasonConfig(ctx) {
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
