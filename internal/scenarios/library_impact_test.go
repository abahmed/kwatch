package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// impactScenarios show who gets paged. Only a failure users feel
// pages: the same crash loop behind an Ingress or LoadBalancer Service
// pages, and behind a ClusterIP Service nobody outside the cluster
// reaches, it notifies.
func impactScenarios() []scenario {
	return []scenario{
		impactScenario("serving-last-replica-down", "page",
			corev1.ServiceTypeLoadBalancer,
			"Every replica of a Deployment behind a LoadBalancer "+
				"Service crash-loops: the Service has no ready "+
				"endpoint and users get errors."),
		impactScenario("internal-only-down", "notify",
			corev1.ServiceTypeClusterIP,
			"Every replica of an internal Deployment behind a "+
				"ClusterIP Service crash-loops: no user-facing "+
				"Service is affected, so nobody is paged."),
	}
}

// impactScenario crash-loops both replicas of the shop Deployment
// "front", which a Service of serviceType selects.
func impactScenario(
	name, tier string, serviceType corev1.ServiceType, description string,
) scenario {
	return scenario{
		expect: expectation{
			Name: name, Description: description,
			Root: "deployment/shop/front", Tier: tier, MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "service/shop/front"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "front",
				"registry.example.com/front:4", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			service := clusterService(c, "shop", "front", 8080)
			service.Spec.Type = serviceType
			if serviceType == corev1.ServiceTypeLoadBalancer {
				service.Status.LoadBalancer.Ingress =
					[]corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
			}
			c.list(service, trafficSlice(c, "front", pods...))
			c.after(time.Minute)
			for restarts := int32(1); restarts <= 5; restarts++ {
				for i := range pods {
					c.update(w.pod(i, "n1", crashLoop(1, "Error",
						"panic: config: listen address is empty",
						restarts)))
				}
				w.setReady(0)
				c.update(w.objects())
				c.update(trafficUnreadySlice(c, "front", pods...))
				c.after(45 * time.Second)
			}
		},
	}
}
