package scenarios

import (
	"time"
)

// serviceSelectorScenarios are Services that no pod serves behind an
// Ingress: a selector with a typo is a failure from the start, while a
// workload scaled to zero by its autoscaler is waiting for traffic.
func serviceSelectorScenarios() []scenario {
	return []scenario{ingressToTypoSelector(), ingressToScaledToZeroService()}
}

// ingressToTypoSelector: the API's pods are healthy but a Service
// selects app=apii, which no pod or pod template carries, and an
// Ingress sends the shop's traffic to it.
func ingressToTypoSelector() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-to-typo-selector",
			Description: "An Ingress routes to a Service whose selector " +
				"has a typo: no pod or workload template carries it.",
			Root: "service/shop/api", Tier: "page", MaxMessages: 2,
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:1.0", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n1"))
			c.after(time.Hour)
			svc := clusterService(c, "shop", "api", 8080)
			svc.Spec.Selector = map[string]string{"app": c.n("apii")}
			c.create(svc, clusterSlice(c, "shop", "api"))
			c.create(trafficIngress(c, "api", ""))
		},
	}
}

// ingressToScaledToZeroService: the twin whose selector is right and
// whose Deployment an autoscaler may scale to zero. The Service is empty
// because nobody called it, not because it selects nothing.
func ingressToScaledToZeroService() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-to-scaled-to-zero-service",
			Description: "An Ingress routes to the Service of a " +
				"Deployment an autoscaler has scaled to zero.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "api",
				"registry.example.com/api:1.0", 0)
			hpa := clusterHPA(c, "shop", "api", "False", "ScalingDisabled",
				"scaling is disabled since the replica count of the "+
					"target is zero")
			zero := int32(0)
			hpa.Spec.MinReplicas = &zero
			hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas = 0, 0
			c.list(w.objects())
			c.list(hpa)
			c.list(clusterService(c, "shop", "api", 8080),
				clusterSlice(c, "shop", "api"))
			c.after(time.Hour)
			c.create(trafficIngress(c, "api", ""))
		},
	}
}
