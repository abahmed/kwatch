package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrEphemeral marks a container added to a running pod to debug it
// (kubectl debug). It is not part of the workload: nothing that judges
// the pod counts it, and its own failures are the debugging person's.
const AttrEphemeral = "ephemeral"

// ephemeralDescriptions describes the pod's ephemeral containers, so
// the warnings about them land on them and not on the pod. They are
// not PartOf the pod: what reads the pod's containers judges the
// workload, which a debug session is not.
func ephemeralDescriptions(pod *corev1.Pod) []Description {
	statuses := make(map[string]corev1.ContainerStatus)
	for _, s := range pod.Status.EphemeralContainerStatuses {
		statuses[s.Name] = s
	}
	out := make([]Description, 0, len(pod.Spec.EphemeralContainers))
	for _, ec := range pod.Spec.EphemeralContainers {
		c := corev1.Container(ec.EphemeralContainerCommon)
		attrs := map[string]inventory.Value{
			AttrEphemeral: inventory.Bool(true),
			AttrImage:     inventory.Text(c.Image),
		}
		if status, ok := statuses[c.Name]; ok {
			containerStatusAttributes(attrs, status)
		}
		out = append(out, Description{
			ID:         ContainerID(pod.Namespace, pod.Name, c.Name),
			Attributes: attrs,
		})
	}
	return out
}
