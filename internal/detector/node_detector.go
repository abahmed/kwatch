package detector

import (
	corev1 "k8s.io/api/core/v1"
)

// NodeDetector detects issues in node status
type NodeDetector struct{}

func NewNodeDetector() *NodeDetector {
	return &NodeDetector{}
}

func (d *NodeDetector) Name() string {
	return "NodeDetector"
}

func (d *NodeDetector) Detect(input *Input) bool {
	if input.Node == nil {
		return false
	}

	for _, c := range input.Node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionFalse {
			input.HasIssue = true
			input.IssueType = "node"
			input.Reason = c.Reason
			input.Message = c.Message
			return true
		}
	}

	return false
}
