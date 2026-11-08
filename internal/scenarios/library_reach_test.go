package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// reachScenarios are incidents that cut users off from an address an
// Ingress publishes. The message names the addresses.
func userReachScenarios() []scenario {
	return []scenario{userEndpointsNamed()}
}

// userEndpointsNamed: a bad release crash-loops every payments pod; the
// Service loses its ready endpoints and the shop Ingress, which routes
// three addresses to it, cannot serve them. The release is the root and
// the message names where users are cut off.
func userEndpointsNamed() scenario {
	return scenario{
		expect: expectation{
			Name: "user-endpoints-named",
			Description: "A release crash-loops every pod behind a " +
				"Service that an Ingress publishes at three addresses.",
			Root: "deployment/shop/payments", Tier: "page",
			MaxMessages: 2, MustNotBlame: []string{"service/shop/payments"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "payments",
				"registry.example.com/payments:3", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "payments", 8080),
				trafficSlice(c, "payments", pods...),
				reachIngress(c, "payments"))
			c.after(time.Minute)
			rs := w.rollout(func(spec *corev1.PodSpec) {
				spec.Containers[0].Image = "registry.example.com/payments:4"
			})
			c.update(w.deployment)
			c.create(rs)
			c.remove(pods[0], pods[1])
			fresh := []*corev1.Pod{w.pod(0, "n1", startedNow),
				w.pod(1, "n1", startedNow)}
			c.create(fresh[0], fresh[1])
			c.after(20 * time.Second)
			unready := trafficUnreadySlice(c, "payments", fresh...)
			for restarts := int32(1); restarts <= 5; restarts++ {
				c.update(w.pod(0, "n1", crashLoop(1, "Error",
					"panic: runtime error: invalid memory address", restarts)),
					w.pod(1, "n1", crashLoop(1, "Error",
						"panic: runtime error: invalid memory address",
						restarts)))
				w.setReady(0)
				c.update(w.objects())
				c.update(unready)
				c.after(45 * time.Second)
			}
		},
	}
}

// reachIngress publishes the backend Service at shop.example.com, at
// /pay and at /pay/refunds, and at a second host.
func reachIngress(c *cluster, backend string) *networkingv1.Ingress {
	prefix := networkingv1.PathTypePrefix
	service := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: c.n(backend),
			Port: networkingv1.ServiceBackendPort{Number: 8080}}}
	path := func(p string) networkingv1.HTTPIngressPath {
		return networkingv1.HTTPIngressPath{Path: p, PathType: &prefix,
			Backend: service}
	}
	rule := func(host string, paths ...networkingv1.HTTPIngressPath,
	) networkingv1.IngressRule {
		return networkingv1.IngressRule{Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths}}}
	}
	return &networkingv1.Ingress{
		ObjectMeta: clusterMeta(c, "shop", "storefront", "ingress"),
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{
			rule("shop.example.com", path("/"), path("/pay"),
				path("/pay/refunds")),
			rule("pay.example.com", path("/")),
		}},
	}
}
