package scenarios

import (
	"time"
)

// digestFlapScenarios are digest-tier problems that come back: the same
// incident must speak for the return, not a new one.
func digestFlapScenarios() []scenario {
	return []scenario{digestFlapReopens()}
}

// digestFlapReopens: an autoscaler that cannot find its target is a
// digest-tier incident. A digest names it, it recovers, and it fails
// again within two hours. The return is the same incident, and the next
// digest lists it once as recurring, instead of opening a new incident
// and listing the old one as resolved.
func digestFlapReopens() scenario {
	return scenario{
		expect: expectation{
			Name: "digest-flap-reopens",
			Description: "A digest-tier autoscaler problem is listed in " +
				"a digest, recovers, and fails again within two hours.",
			Root: "horizontalpodautoscaler/shop/orders", Tier: "digest",
			MaxMessages: 3, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			message := "the HPA controller was unable to get the " +
				"target's current scale: deployments.apps \"orders\" " +
				"not found"
			hpa := clusterHPA(c, "shop", "orders", "False",
				"FailedGetScale", message)
			c.list(hpa)
			failedScale := func(first int32) {
				for n := first; n < first+4; n++ {
					c.after(2 * time.Minute)
					c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
						"FailedGetScale", message,
						"horizontal-pod-autoscaler", n))
				}
			}
			failedScale(1)
			c.after(35 * time.Minute)
			orders := c.deployment("shop", "orders",
				"registry.example.com/orders:5", 2)
			c.create(orders.objects())
			c.update(clusterHPA(c, "shop", "orders", "True",
				"SucceededGetScale", "the HPA controller was able to "+
					"get the target's current scale"))
			c.after(20 * time.Minute)
			c.remove(orders.deployment, orders.replicaSet)
			c.update(clusterHPA(c, "shop", "orders", "False",
				"FailedGetScale", message))
			failedScale(5)
		},
	}
}
