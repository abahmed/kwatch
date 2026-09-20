package pod

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

const stuckDeletionGrace = 10 * time.Minute

// DetectDeletionIssue reports a Pod that remains terminating while finalizer
// cleanup is stuck beyond the grace period.
func DetectDeletionIssue(
	pod *corev1.Pod,
	now time.Time,
) *model.Observation {
	if pod == nil || pod.DeletionTimestamp == nil ||
		len(pod.Finalizers) == 0 {
		return nil
	}
	if now.Sub(pod.DeletionTimestamp.Time) < stuckDeletionGrace {
		return nil
	}
	return observe.PodOwnedBy(
		pod, "", constant.ReasonPodStuckTerminating,
		model.ObjectRef{},
	).WithHint(fmt.Sprintf(
		"pod has been terminating for %s with finalizers: %s",
		now.Sub(pod.DeletionTimestamp.Time).Round(time.Minute),
		strings.Join(pod.Finalizers, ", "),
	))
}
