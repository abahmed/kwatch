package filter

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
)

type PodStatusFilter struct{}

func (f PodStatusFilter) Execute(ctx *Context) bool {
	if ctx.Pod.Status.Phase == corev1.PodSucceeded {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return true
	}

	if ctx.EvType == "Added" && len(ctx.Pod.Status.Conditions) == 0 {
		ctx.PodHasIssues = false
		ctx.ContainersHasIssues = false
		return true
	}

	issueInContainers := true
	issueInPod := true
	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			if c.Status == corev1.ConditionFalse && c.Reason == "PodCompleted" {
				ctx.PodHasIssues = false
				ctx.ContainersHasIssues = false
				return true
			}

			issueInPod = false
			issueInContainers = false
			if c.Status != corev1.ConditionTrue {
				issueInContainers = true
			}
		} else if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			issueInPod = true
			issueInContainers = false
			ctx.PodReason = c.Reason
			ctx.PodMsg = c.Message
		} else if c.Type == corev1.ContainersReady && c.Status == corev1.ConditionFalse {
			issueInContainers = true
			issueInPod = false
		}
	}

	ctx.PodHasIssues = issueInPod
	ctx.ContainersHasIssues = issueInContainers

	if len(ctx.PodReason) > 0 &&
		len(ctx.Config.AllowedReasons) > 0 &&
		!slices.Contains(ctx.Config.AllowedReasons, ctx.PodReason) {
		logrus.Infof(
			"skipping reason %s for pod %s as it is not in the reason allow list",
			ctx.PodReason,
			ctx.Pod.Name)
		return true
	}

	if len(ctx.PodReason) > 0 &&
		len(ctx.Config.ForbiddenReasons) > 0 &&
		slices.Contains(ctx.Config.ForbiddenReasons, ctx.PodReason) {
		logrus.Infof(
			"skipping reason %s for pod %s as it is in the reason forbid list",
			ctx.PodReason,
			ctx.Pod.Name)
		return true
	}

	lastState := ctx.Memory.GetPodContainer(ctx.Pod.Namespace,
		ctx.Pod.Name,
		".")

	if ctx.PodHasIssues && lastState != nil {
		return true
	}

	return false
}
