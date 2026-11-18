package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrSidecarStarting is true for a sidecar (an init container with
// restartPolicy Always) that runs but has not passed its startup probe
// yet. The pod stays Pending and its app containers wait until it has:
// the pod is starting, not stuck in an init container. Absent otherwise.
const AttrSidecarStarting = "sidecar.starting"

func setSidecarStarting(
	attrs map[string]inventory.Value, s corev1.ContainerStatus,
) {
	if flagAttr(attrs, AttrSidecar) && s.State.Running != nil &&
		s.Started != nil && !*s.Started {
		attrs[AttrSidecarStarting] = inventory.Bool(true)
	}
}
