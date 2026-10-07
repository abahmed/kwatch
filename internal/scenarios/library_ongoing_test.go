package scenarios

import "time"

// ongoingScenarios are digest-tier problems that stay open.
func ongoingScenarios() []scenario {
	return []scenario{digestOngoing()}
}

// digestOngoing: Kubernetes reports the same load balancer failure on a
// Service every sixteen minutes for hours. The first digest names it
// once; without more it would never be mentioned again. The digest that
// follows hours later lists it as still failing, with how often it was
// seen and since when.
func digestOngoing() scenario {
	return scenario{
		expect: expectation{
			Name: "digest-ongoing",
			Description: "A Service gets the same FailedDeployModel " +
				"warning every sixteen minutes for eight hours.",
			Root: "service/kube-system/ingress-internal", Tier: "digest",
			MaxMessages: 3, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			svc := clusterService(c, "kube-system", "ingress-internal", 80)
			c.list(svc)
			message := "Failed deploy model due to DependencyViolation: " +
				"the load balancer cannot be deleted"
			for n := int32(1); n <= 30; n++ {
				c.warn(c.warningEvent(svc, "Service", "FailedDeployModel",
					message, "service", n))
				c.after(16 * time.Minute)
			}
		},
	}
}
