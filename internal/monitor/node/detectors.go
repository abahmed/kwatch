package node

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	corev1 "k8s.io/api/core/v1"
)

const (
	newNodeGracePeriod     = 5 * time.Minute
	stuckNodeDeletionGrace = 10 * time.Minute
)

// IsNew reports whether a Node is still within its bootstrap grace period.
func IsNew(node *corev1.Node, now time.Time) bool {
	return node != nil &&
		now.Sub(node.CreationTimestamp.Time) < newNodeGracePeriod
}

// ConditionReason returns the stable incident reason for a Node condition.
func ConditionReason(condition corev1.NodeCondition) string {
	switch condition.Type {
	case corev1.NodeReady:
		if condition.Status != corev1.ConditionTrue {
			return constant.ReasonNodeNotReady
		}
	case corev1.NodeMemoryPressure, corev1.NodeDiskPressure,
		corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
		if condition.Status == corev1.ConditionTrue {
			return string(condition.Type)
		}
	}
	return ""
}

// DetectDeletionIssue reports a Node stuck terminating behind finalizers.
func DetectDeletionIssue(
	node *corev1.Node,
	now time.Time,
) *model.Observation {
	if node == nil || node.DeletionTimestamp == nil ||
		len(node.Finalizers) == 0 {
		return nil
	}
	age := now.Sub(node.DeletionTimestamp.Time)
	if age < stuckNodeDeletionGrace {
		return nil
	}
	return observe.Node(node, constant.ReasonNodeStuckTerminating).
		WithHint(fmt.Sprintf(
			"node %s has been terminating for %s with finalizers: %s",
			node.Name,
			age.Round(time.Minute),
			strings.Join(node.Finalizers, ", "),
		))
}
