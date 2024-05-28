package filter

import (
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

	lastState := ctx.Memory.GetPodContainer(ctx.Pod.Namespace,
		ctx.Pod.Name,
		".")

	if ctx.PodHasIssues && lastState != nil {
		return true
	}

	return false
}
