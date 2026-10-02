package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// trafficScenarios are failures on the way requests reach pods: a
// Service, an Ingress or a Gateway API route broken by an edit, and the
// negative case where a Service only restates its crashing pods.
func trafficScenarios() []scenario {
	return []scenario{
		serviceSelectorChange(), serviceBackendsCrash(),
		ingressBackendMissing(), ingressTLSSecretMissing(),
		routeNotAccepted(),
	}
}

// serviceSelectorChange: the payments Service's selector is edited to a
// typo; it selects no pod any more while every payments pod is healthy.
// The Service is the root, not the pods or their Deployment.
func serviceSelectorChange() scenario {
	return scenario{
		expect: expectation{
			Name: "service-selector-change",
			Description: "A Service's selector is edited to a typo; it " +
				"has no endpoints while its pods stay healthy.",
			Root: "service/shop/payments", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/payments"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "payments",
				"registry.example.com/payments:3", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			svc := clusterService(c, "shop", "payments", 8080)
			c.list(svc, trafficSlice(c, "payments", pods...))
			c.after(time.Minute)
			edited := last(c, svc)
			edited.Spec.Selector = map[string]string{
				"app": c.n("payment"),
			}
			c.update(edited)
			c.update(trafficSlice(c, "payments"))
			c.after(5 * time.Minute)
		},
	}
}

// serviceBackendsCrash: a bad release makes every payments pod crash;
// the Service loses its ready endpoints as a consequence. The release is
// the root; the Service only restates its pods and must not be blamed.
func serviceBackendsCrash() scenario {
	return scenario{
		expect: expectation{
			Name: "service-backends-crash",
			Description: "A release crash-loops every pod behind a " +
				"Service; the Service has no ready endpoints.",
			Root: "deployment/shop/payments", Tier: "notify",
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
				trafficSlice(c, "payments", pods...))
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

// ingressBackendMissing: the shop Ingress is edited to send traffic to
// Service web-v2, which was never created. The missing Service is the
// root, as a missing Secret is for the pods that name it.
func ingressBackendMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-backend-missing",
			Description: "An Ingress is edited to route to a Service " +
				"that does not exist.",
			Root: "service/shop/web-v2", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{"service/shop/web",
				"deployment/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			ingress := trafficIngress(c, "web", "")
			c.list(ingress)
			c.after(time.Minute)
			c.update(trafficIngress(c, "web-v2", ""))
			c.after(5 * time.Minute)
		},
	}
}

// ingressTLSSecretMissing: the shop Ingress is given a TLS section that
// names a Secret nobody created. The missing Secret is the root.
func ingressTLSSecretMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "ingress-tls-secret-missing",
			Description: "An Ingress terminates TLS with a Secret that " +
				"does not exist.",
			Root: "secret/shop/web-tls", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"service/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			c.list(trafficIngress(c, "web", ""))
			c.after(time.Minute)
			c.update(trafficIngress(c, "web", "web-tls"))
			c.after(5 * time.Minute)
		},
	}
}

// routeNotAccepted: an HTTPRoute is edited to attach to a listener its
// Gateway does not have; the Gateway stops accepting it. The edited
// route is the root, not the Gateway or the backend.
func routeNotAccepted() scenario {
	return scenario{
		expect: expectation{
			Name: "route-not-accepted",
			Description: "An HTTPRoute edit names a listener the Gateway " +
				"does not have; the route is no longer accepted.",
			Root:        "httproute.gateway.networking.k8s.io/shop/web",
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: []string{
				"gateway.gateway.networking.k8s.io/shop/public",
				"service/shop/web"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
			c.list(w.objects())
			pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
			c.list(pods[0], pods[1])
			c.list(clusterService(c, "shop", "web", 8080),
				trafficSlice(c, "web", pods...))
			c.list(trafficGateway(c), trafficRoute(c, 1, "https", "True",
				"Accepted", ""))
			c.after(time.Minute)
			c.update(trafficRoute(c, 2, "http-internal", "False",
				"NoMatchingParent", "No listener named http-internal"))
			c.after(5 * time.Minute)
		},
	}
}

// trafficSlice is the shop Service's EndpointSlice, publishing the
// 8080 target port the pods listen on.
func trafficSlice(
	c *cluster, service string, pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, "shop", service, pods...)
	port := int32(8080)
	slice.Ports = []discoveryv1.EndpointPort{{Port: &port}}
	return slice
}

// trafficUnreadySlice lists the pods as unready endpoints of a Service.
func trafficUnreadySlice(
	c *cluster, service string, pods ...*corev1.Pod,
) *discoveryv1.EndpointSlice {
	slice := trafficSlice(c, service, pods...)
	for i := range slice.Endpoints {
		slice.Endpoints[i].Conditions.Ready = boolPtr(false)
	}
	return slice
}

// trafficIngress routes the shop Ingress to backend, terminating TLS
// with tlsSecret when it is set.
func trafficIngress(c *cluster, backend, tlsSecret string,
) *networkingv1.Ingress {
	prefix := networkingv1.PathTypePrefix
	ing := &networkingv1.Ingress{
		ObjectMeta: clusterMeta(c, "shop", "shop", "ingress"),
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: "shop.example.com",
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path: "/", PathType: &prefix,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: c.n(backend),
								Port: networkingv1.ServiceBackendPort{
									Number: 8080},
							},
						},
					}},
				},
			},
		}}},
	}
	if tlsSecret != "" {
		ing.Spec.TLS = []networkingv1.IngressTLS{{
			Hosts: []string{"shop.example.com"}, SecretName: c.n(tlsSecret),
		}}
	}
	return ing
}

func trafficGateway(c *cluster) *unstructured.Unstructured {
	meta := clusterMeta(c, "shop", "public", "gateway")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
		"spec": map[string]any{"gatewayClassName": "envoy",
			"listeners": []any{map[string]any{"name": "https",
				"port": int64(443), "protocol": "HTTPS"}}},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	return u
}

// trafficRoute is the web HTTPRoute at generation, attached to listener
// of the public Gateway, with its Accepted condition from that Gateway.
func trafficRoute(
	c *cluster, generation int64, listener, accepted, reason,
	message string,
) *unstructured.Unstructured {
	meta := clusterMeta(c, "shop", "web", "route")
	parent := map[string]any{"name": c.n("public"), "sectionName": listener}
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"spec": map[string]any{
			"parentRefs": []any{parent},
			"rules": []any{map[string]any{"backendRefs": []any{
				map[string]any{"name": c.n("web"), "port": int64(8080)}}}},
		},
		"status": map[string]any{"parents": []any{map[string]any{
			"parentRef": parent, "controllerName": "envoy",
			"conditions": []any{map[string]any{
				"type": "Accepted", "status": accepted, "reason": reason,
				"message":            message,
				"observedGeneration": generation,
				"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
			}},
		}}},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(generation)
	return u
}
