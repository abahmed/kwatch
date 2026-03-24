package detector

import (
	corev1 "k8s.io/api/core/v1"
)

// PodDetector detects issues in pod status
type PodDetector struct{}

func NewPodDetector() *PodDetector {
	return &PodDetector{}
}

func (d *PodDetector) Name() string {
	return "PodDetector"
}

func (d *PodDetector) Detect(input *Input) bool {
	if input.Pod == nil {
		return false
	}

	phase := input.Pod.Status.Phase

	// Check phase
	if phase == corev1.PodSucceeded {
		return false
	}

	if phase == corev1.PodFailed {
		input.HasIssue = true
		input.IssueType = "pod"
		input.Reason = string(phase)
		input.Message = input.Pod.Status.Message
		return true
	}

	// Check pod conditions
	for _, c := range input.Pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionFalse {
			input.HasIssue = true
			input.IssueType = "pod"
			input.Reason = c.Reason
			input.Message = c.Message
			return true
		}

		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			input.HasIssue = true
			input.IssueType = "pod"
			input.Reason = c.Reason
			input.Message = c.Message
			return true
		}

		if c.Type == corev1.ContainersReady && c.Status == corev1.ConditionFalse {
			input.HasIssue = true
			input.IssueType = "pod"
			input.Reason = c.Reason
			input.Message = c.Message
			return true
		}
	}

	return false
}
