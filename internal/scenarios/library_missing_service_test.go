package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// missingServiceScenarios are workloads that call a Service of their own
// cluster that does not exist.
func missingServiceScenarios() []scenario {
	return []scenario{missingServiceLookup(), missingServiceUnused()}
}

// paymentsClient lists the shop API, which reads the payments address
// from PAYMENTS_URL. The name is misspelt: no Service paymnts exists,
// while Service payments does.
func paymentsClient(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"))
	payments := c.deployment("shop", "payments",
		"registry.example.com/payments:4", 1)
	c.list(payments.objects())
	c.list(payments.pod(0, "n1"), clusterService(c, "shop", "payments", 8080),
		trafficSlice(c, "payments", payments.pod(0, "n1")))
	api := c.deployment("shop", "api", "registry.example.com/api:5", 2)
	configTemplate(api, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "PAYMENTS_URL", Value: "http://" + c.n("paymnts") +
				".shop.svc:8080"}}
	})
	c.list(api.objects())
	return api
}

// missingServiceLookup: both API pods crash because the name they look
// up is not a Service. The missing Service is the root.
func missingServiceLookup() scenario {
	return scenario{
		expect: expectation{
			Name: "missing-service-lookup",
			Description: "The API crashes with a failed lookup of " +
				"paymnts.shop.svc; only a Service named payments exists.",
			Root: "service/shop/paymnts", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/payments",
				"node//n1"},
		},
		build: func(c *cluster) {
			api := paymentsClient(c)
			c.list(api.pod(0, "n1"), api.pod(1, "n1"))
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for replica := range 2 {
					c.update(api.pod(replica, "n1", crashLoop(1, "Error",
						"dial tcp: lookup "+c.n("paymnts")+
							".shop.svc.cluster.local: no such host",
						restarts)))
				}
				api.setReady(0)
				c.update(api.objects())
				c.after(time.Minute)
			}
		},
	}
}

// missingServiceUnused: the same misspelt name sits in the API's
// configuration, but the API runs fine: it does not use it yet. Nothing
// is happening, so nothing is said.
func missingServiceUnused() scenario {
	return scenario{
		expect: expectation{
			Name: "missing-service-unused",
			Description: "The API's configuration names a Service that " +
				"does not exist, and the API is healthy.",
			Quiet: true,
		},
		build: func(c *cluster) {
			api := paymentsClient(c)
			c.list(api.pod(0, "n1"), api.pod(1, "n1"))
			c.after(15 * time.Minute)
		},
	}
}
