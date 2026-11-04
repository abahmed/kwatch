package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// inUseDeleteScenarios are controllers that cannot delete an external
// resource because something still uses it.
func inUseDeleteScenarios() []scenario {
	return []scenario{loadBalancerDeleteInUse(), loadBalancerDeleteRetried()}
}

// lbInUseMessage is a load balancer controller's refusal from the
// cloud: the balancer it should delete is still associated with
// another service. The words, not the controller, make the rule.
const lbInUseMessage = "Failed deploy model due to operation error " +
	"load balancing: DeleteLoadBalancer, api error ResourceInUse: load " +
	"balancer lb-7f3 cannot be deleted because it is currently " +
	"associated with another service"

// loadBalancerDeleteInUse: a Service was switched away from its old
// load balancer, and the controller keeps failing to delete the old
// one, which another service still uses. Nothing else fails; someone
// must free or remove the old balancer. It is said at the first
// refusal, in the digest like every controller event kwatch has no
// detector for. It is said at the first
// refusal, in the digest like every controller event kwatch has no
// detector for.
func loadBalancerDeleteInUse() scenario {
	return scenario{
		expect: expectation{
			Name: "lb-delete-resource-in-use",
			Description: "A load balancer controller keeps failing to " +
				"delete a balancer that another service still uses.",
			Root: "service/shop/web", Tier: "digest", MaxMessages: 2,
			MustNotBlame: []string{"node//n1"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			web := inUseService(c)
			for count := int32(1); count <= 4; count++ {
				c.warn(c.warningEvent(web, "Service", "FailedDeployModel",
					lbInUseMessage, "service", count))
				c.after(2 * time.Minute)
			}
		},
	}
}

// loadBalancerDeleteRetried: the controller fails once to delete the
// old balancer because the cloud throttles it, and the retry works.
// Nothing is in use: one retried error is no problem.
func loadBalancerDeleteRetried() scenario {
	return scenario{
		expect: expectation{
			Name: "lb-delete-throttled-once",
			Description: "A load balancer controller's delete is " +
				"throttled once and then succeeds.",
			Quiet: true, Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			web := inUseService(c)
			c.warn(c.warningEvent(web, "Service", "FailedDeployModel",
				"Failed deploy model due to operation error: "+
					"DeleteLoadBalancer, api error Throttling: rate "+
					"exceeded", "service", 1))
			c.after(20 * time.Minute)
		},
	}
}

// inUseService is shop/web, a LoadBalancer Service with an address and
// ready pods, after ten quiet minutes.
func inUseService(c *cluster) *corev1.Service {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:9", 1)
	web := clusterService(c, "shop", "web", 8080)
	web.Spec.Type = corev1.ServiceTypeLoadBalancer
	web.Status.LoadBalancer.Ingress =
		[]corev1.LoadBalancerIngress{{IP: "203.0.113.20"}}
	c.list(w.objects())
	c.list(w.pod(0, "n1"), web, trafficSlice(c, "web", w.pod(0, "n1")))
	c.after(10 * time.Minute)
	return web
}
