package kube

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// metaConditions converts metav1.Conditions, the shape newer built-in
// APIs (PodDisruptionBudget) use.
func metaConditions(in []metav1.Condition) []condition {
	out := make([]condition, 0, len(in))
	for _, c := range in {
		out = append(out, condition{
			Type: c.Type, Status: string(c.Status), Reason: c.Reason,
			Message: c.Message, Since: c.LastTransitionTime.Time,
		})
	}
	return out
}
