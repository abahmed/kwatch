package scenarios

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// limitRangeMemoryRejects: the LimitRange of the shop namespace is
// edited to cap a container at 1Gi of memory; the next pods, which
// limit 2Gi, are refused. The LimitRange is the root.
func limitRangeMemoryRejects() scenario {
	return scenario{
		expect: expectation{
			Name: "limit-range-memory-max-lowered",
			Description: "A LimitRange is edited to cap containers at " +
				"1Gi of memory; a scale-up of a Deployment whose pods " +
				"limit 2Gi is refused.",
			Root: "limitrange/shop/limits", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/shop/app",
				"namespace//shop"},
		},
		build: func(c *cluster) {
			bounds := func(memory string) *corev1.LimitRange {
				return &corev1.LimitRange{
					ObjectMeta: clusterMeta(c, "shop", "limits",
						"limitrange"),
					Spec: corev1.LimitRangeSpec{Limits: []corev1.
						LimitRangeItem{{Type: corev1.LimitTypeContainer,
						Max: corev1.ResourceList{corev1.ResourceMemory: resource.
							MustParse(memory)}}}},
				}
			}
			c.list(bounds("4Gi"))
			policyDenied(c, "shop", "app", func(c *cluster) {
				c.update(bounds("1Gi"))
			}, "maximum memory usage per Container is 1Gi, "+
				"but limit is 2Gi")
		},
	}
}
