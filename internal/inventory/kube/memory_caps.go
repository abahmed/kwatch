package kube

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Namespace bounds a new memory limit must fit, so a step that raises a
// limit is checked before it is offered.
const (
	// AttrLimitMaxMemory is the largest memory limit a LimitRange lets
	// one container have, in bytes; absent without such a bound.
	AttrLimitMaxMemory = "limits.max.memory.bytes"
	// AttrQuotaMemoryLeft is how much of a ResourceQuota's limits.memory
	// is still unused, in bytes; absent when the quota does not bound
	// memory limits.
	AttrQuotaMemoryLeft = "quota.limits.memory.left.bytes"
)

// addLimitMaxMemory records the container memory maximum of lr.
func addLimitMaxMemory(
	attrs map[string]inventory.Value, lr *corev1.LimitRange,
) {
	best := 0.0
	for _, item := range lr.Spec.Limits {
		if item.Type != corev1.LimitTypeContainer {
			continue
		}
		if q, ok := item.Max[corev1.ResourceMemory]; ok {
			v := q.AsApproximateFloat64()
			if v > 0 && (best == 0 || v < best) {
				best = v
			}
		}
	}
	if best > 0 {
		attrs[AttrLimitMaxMemory] = inventory.Number(best)
	}
}

// addQuotaMemoryLeft records the room left under a quota's
// limits.memory.
func addQuotaMemoryLeft(
	attrs map[string]inventory.Value, quota *corev1.ResourceQuota,
) {
	hard, ok := quota.Status.Hard[corev1.ResourceLimitsMemory]
	if !ok {
		return
	}
	used := quota.Status.Used[corev1.ResourceLimitsMemory]
	attrs[AttrQuotaMemoryLeft] = inventory.Number(max(0,
		hard.AsApproximateFloat64()-used.AsApproximateFloat64()))
}
