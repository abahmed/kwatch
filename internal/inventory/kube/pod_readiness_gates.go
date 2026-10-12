package kube

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrReadinessGates lists the condition types in the pod's
// spec.readinessGates, comma separated, in declared order. Each gate's
// status is the pod condition of that type (see ConditionKey); a gate
// with no such condition has not been set by its controller yet. Absent
// when the pod declares no gate.
const AttrReadinessGates = "pod.readiness.gates"

// setReadinessGates records the readiness gates the pod declares: a
// pod stays NotReady until every gate condition is True, although its
// containers are ready.
func setReadinessGates(
	attrs map[string]inventory.Value, pod *corev1.Pod,
) {
	gates := make([]string, 0, len(pod.Spec.ReadinessGates))
	for _, gate := range pod.Spec.ReadinessGates {
		if gate.ConditionType != "" {
			gates = append(gates, string(gate.ConditionType))
		}
	}
	if len(gates) > 0 {
		attrs[AttrReadinessGates] = inventory.Text(strings.Join(gates, ","))
	}
}
