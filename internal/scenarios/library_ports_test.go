package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// portScenarios are Services and Ingresses that send traffic to a port
// that nothing serves.
func portScenarios() []scenario {
	return []scenario{
		serviceTargetPortNameMissing(), serviceTargetPortUndeclared(),
		ingressBackendPortMissing(),
	}
}

// apiWithPorts is the shop api Deployment whose container declares port
// http (8080), with its two pods.
func apiWithPorts(c *cluster) (*workload, []*corev1.Pod) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "api", "registry.example.com/api:2", 2)
	ports := []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}
	w.deployment.Spec.Template.Spec.Containers[0].Ports = ports
	w.replicaSet.Spec.Template.Spec.Containers[0].Ports = ports
	c.list(w.objects())
	pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
	c.list(pods[0], pods[1])
	return w, pods
}

// serviceTargetPortNameMissing: the api Service is edited to target the
// port named "web", but its pods only declare "http". Its endpoints
// vanish while every pod is healthy; the Service is the root.
func serviceTargetPortNameMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "service-target-port-name-missing",
			Description: "A Service is edited to target a named port " +
				"that none of its pods declare.",
			Root: "service/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api"},
		},
		build: func(c *cluster) {
			_, pods := apiWithPorts(c)
			svc := clusterService(c, "shop", "api", 80)
			svc.Spec.Ports[0].TargetPort = intstr.FromString("http")
			c.list(svc, trafficSlice(c, "api", pods...))
			c.after(time.Minute)
			edited := last(c, svc)
			edited.Spec.Ports[0].TargetPort = intstr.FromString("web")
			c.update(edited)
			c.update(trafficSlice(c, "api"))
			c.after(5 * time.Minute)
		},
	}
}

// serviceTargetPortUndeclared: the Service targets 8081 while its pods
// declare only 8080, but its endpoints are ready and nothing is refused.
// Declared ports are informational, so kwatch stays silent.
func serviceTargetPortUndeclared() scenario {
	return scenario{
		expect: expectation{
			Name: "service-target-port-undeclared",
			Description: "A Service targets a number its pods do not " +
				"declare, yet its endpoints are ready and nothing fails.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			_, pods := apiWithPorts(c)
			svc := clusterService(c, "shop", "api", 80)
			svc.Spec.Ports[0].TargetPort = intstr.FromInt32(8081)
			c.list(svc, trafficSlice(c, "api", pods...))
			c.after(10 * time.Minute)
		},
	}
}

// ingressBackendPortMissing: the shop Ingress is edited to send /api to
// port 9090 of the api Service, which only has 8080.
func ingressBackendPortMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-backend-port-missing",
			Description: "An Ingress is edited to route to a Service " +
				"port the Service does not have.",
			Root: "ingress/shop/shop", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"service/shop/api",
				"deployment/shop/api"},
		},
		build: func(c *cluster) {
			_, pods := apiWithPorts(c)
			c.list(clusterService(c, "shop", "api", 8080),
				trafficSlice(c, "api", pods...))
			c.list(trafficIngress(c, "api", ""))
			c.after(time.Minute)
			edited := trafficIngress(c, "api", "")
			path := &edited.Spec.Rules[0].HTTP.Paths[0]
			path.Path = "/api"
			path.Backend.Service.Port.Number = 9090
			c.update(edited)
			c.after(5 * time.Minute)
		},
	}
}
