package scenarios

import (
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// scaleZeroScenarios are workloads scaled to zero replicas.
func scaleZeroScenarios() []scenario {
	return []scenario{scaledToZeroRouted(), scaledToZeroUnrouted(),
		scaledToZeroBlueGreen()}
}

// scaledToZeroRouted: someone scales the shop API to zero while the
// shop Ingress still routes to its Service.
func scaledToZeroRouted() scenario {
	return scenario{
		expect: expectation{
			Name: "scaled-to-zero-routed",
			Description: "A Deployment is scaled to zero replicas by " +
				"kubectl-scale while an Ingress still routes to its " +
				"Service.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildScaledToZero(c, true, false) },
	}
}

// scaledToZeroUnrouted: the same scale-down, but nothing routes to the
// Service: a paused workload nobody calls is not news.
func scaledToZeroUnrouted() scenario {
	return scenario{
		expect: expectation{
			Name: "scaled-to-zero-unrouted",
			Description: "A Deployment is scaled to zero replicas and no " +
				"Ingress routes to its Service.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildScaledToZero(c, false, false) },
	}
}

// scaledToZeroBlueGreen: the blue Deployment is scaled to zero while an
// Ingress still routes to its Service, but the green Deployment behind
// the same selector keeps the Service's endpoints ready: traffic is
// still answered, so there is nothing to report.
func scaledToZeroBlueGreen() scenario {
	return scenario{
		expect: expectation{
			Name: "scaled-to-zero-blue-green",
			Description: "A Deployment is scaled to zero while an " +
				"Ingress routes to its Service, and another Deployment " +
				"behind the same Service keeps its endpoints ready.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) { buildScaledToZero(c, true, true) },
	}
}

func buildScaledToZero(c *cluster, routed, otherServes bool) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:1.0", 3)
	c.list(w.objects())
	c.list(clusterService(c, "shop", "api", 8080))
	if routed {
		c.list(trafficIngress(c, "api", ""))
	}
	if otherServes {
		green := c.deployment("shop", "api-green",
			"registry.example.com/api:2.0", 1)
		c.list(green.objects())
		slice := clusterSlice(c, "shop", "api", green.pod(0, "n1"))
		port := int32(8080)
		slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
		c.list(slice)
	}
	c.after(30 * time.Minute)
	zero := int32(0)
	scaled := w.deployment.DeepCopy()
	scaled.Spec.Replicas = &zero
	scaled.Status.Replicas, scaled.Status.ReadyReplicas = 0, 0
	scaled.Status.AvailableReplicas, scaled.Status.UpdatedReplicas = 0, 0
	scaled.ManagedFields = []metav1.ManagedFieldsEntry{{
		Manager: "kubectl-scale", Operation: metav1.ManagedFieldsOperationUpdate,
		Time: &metav1.Time{Time: c.now},
	}}
	c.update(scaled)
}
