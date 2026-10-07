package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// loadBalancerScenarios are LoadBalancer Services whose address never
// arrives.
func loadBalancerScenarios() []scenario {
	return []scenario{loadBalancerSubnets(), loadBalancerNoController()}
}

// loadBalancerSubnets: the Service behind the shop Ingress asked for a
// load balancer minutes ago. The controller keeps failing to build
// it. The Service is the root, the controller's own words are quoted,
// and because the Ingress depends on it, it notifies.
func loadBalancerSubnets() scenario {
	return scenario{
		expect: expectation{
			Name: "loadbalancer-subnets-not-found",
			Description: "A LoadBalancer Service behind an Ingress has " +
				"no address for minutes; the controller fails to " +
				"build its model.",
			Root: "service/shop/web", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			web := c.deployment("shop", "web",
				"registry.example.com/web:3", 2)
			c.list(web.objects())
			pods := []*corev1.Pod{web.pod(0, "n1"), web.pod(1, "n2")}
			c.list(pods[0], pods[1])
			svc := clusterService(c, "shop", "web", 8080)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			c.list(svc)
			c.list(clusterSlice(c, "shop", "web", pods...))
			c.list(trafficIngress(c, "web", ""))
			c.warn(c.normalEvent(svc, "EnsuringLoadBalancer",
				"Ensuring load balancer", "service-controller", 1))
			c.after(2 * time.Minute)
			message := "Failed build model due to couldn't auto-discover " +
				"subnets: unable to resolve at least one subnet (0 " +
				"match VPC and tags: [kubernetes.io/role/elb])"
			for n := int32(1); n <= 4; n++ {
				c.warn(c.warningEvent(svc, "Service", "FailedBuildModel",
					message, "service", n))
				c.after(3 * time.Minute)
			}
		},
	}
}

// loadBalancerNoController: a new LoadBalancer Service names a class no
// controller implements, so nothing ever reacts to it. Nobody depends on
// it yet: it waits in the digest.
func loadBalancerNoController() scenario {
	return scenario{
		expect: expectation{
			Name: "loadbalancer-no-controller",
			Description: "A new LoadBalancer Service names a load " +
				"balancer class and no controller has acted on it.",
			Root: "service/shop/api", Tier: "digest", MaxMessages: 2,
			Tail: duration(20 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			api := c.deployment("shop", "api",
				"registry.example.com/api:5", 1)
			c.list(api.objects())
			pod := api.pod(0, "n1")
			c.list(pod)
			svc := clusterService(c, "shop", "api", 8080)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			class := "example.com/internal-lb"
			svc.Spec.LoadBalancerClass = &class
			c.list(svc)
			c.list(clusterSlice(c, "shop", "api", pod))
			c.after(15 * time.Minute)
		},
	}
}
