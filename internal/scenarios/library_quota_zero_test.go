package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// clusterQuotaForbidsPods: a ResourceQuota allows zero pods, so the
// ReplicaSet creates none. A zero limit alone says nothing, but the refused
// creates name this quota, so it is the root.
func clusterQuotaForbidsPods() scenario {
	return scenario{
		expect: expectation{
			Name: "quota-forbids-pods",
			Description: "A ReplicaSet cannot create pods in a namespace " +
				"whose ResourceQuota allows none.",
			Root: "resourcequota/analytics/zero-pods", Tier: "notify",
			MaxMessages: 2,
		},
		build: buildClusterQuotaForbidsPods,
	}
}

func buildClusterQuotaForbidsPods(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("analytics", "ingest",
		"registry.example.com/ingest:4.0", 2)
	w.setReady(0)
	c.list(w.objects())
	none := corev1.ResourceList{
		corev1.ResourcePods: resource.MustParse("0"),
	}
	quota := &corev1.ResourceQuota{
		ObjectMeta: clusterMeta(c, "analytics", "zero-pods", "quota"),
		Spec:       corev1.ResourceQuotaSpec{Hard: none},
		Status:     corev1.ResourceQuotaStatus{Hard: none, Used: none},
	}
	c.list(quota)
	for n := int32(1); n <= 5; n++ {
		message := fmt.Sprintf("Error creating: pods \"%s-%c\" is "+
			"forbidden: exceeded quota: %s, requested: pods=1, "+
			"used: pods=0, limited: pods=0",
			w.replicaSet.Name, 'e'+rune(n), quota.Name)
		c.warn(c.warningEvent(w.replicaSet, "ReplicaSet", "FailedCreate",
			message, "replicaset-controller", n))
		c.after(time.Minute)
	}
}
