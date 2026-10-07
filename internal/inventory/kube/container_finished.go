package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrFinished is when the container's current terminated state ended.
// It tells a container that exited when its pod's grace period ran out
// from one that stopped early.
const AttrFinished = "finished"

func setFinished(
	attrs map[string]inventory.Value, t *corev1.ContainerStateTerminated,
) {
	if !t.FinishedAt.IsZero() {
		attrs[AttrFinished] = inventory.Time(t.FinishedAt.Time)
	}
}
