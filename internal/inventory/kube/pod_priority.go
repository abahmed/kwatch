package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrPriority is the pod's scheduling priority (spec.priority), set
// when the API server resolved a priority class. It lets a message say
// that a preemptor outranked its victim.
const AttrPriority = "priority"

// PreemptedReason is the reason of the scheduler's event on a victim:
// "Preempted by <namespace>/<name> on node <node>" (older releases) or
// "Preempted by pod <uid> on node <node>".
const PreemptedReason = "Preempted"

func setPriority(attrs map[string]inventory.Value, pod *corev1.Pod) {
	if pod.Spec.Priority != nil {
		attrs[AttrPriority] = inventory.Number(float64(*pod.Spec.Priority))
	}
}
