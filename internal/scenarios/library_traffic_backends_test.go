package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// trafficBackendScenarios are Gateway API routes whose backend Service
// does not exist.
func trafficBackendScenarios() []scenario {
	return []scenario{
		routeBackendRenamed(), routeCanaryBackendMissing(),
		routeMissingBackend(),
	}
}

// routeBackendRenamed: the storefront api HTTPRoute is edited to send
// traffic to Service search-v3, which was never created. The Gateway
// accepts the route but reports ResolvedRefs False (BackendNotFound).
// The missing Service is the root: the route's edit is how it was
// named, but what a person must create or correct is the Service.
func routeBackendRenamed() scenario {
	return scenario{
		expect: expectation{
			Name: "route-backend-renamed",
			Description: "An HTTPRoute edit names a backend Service that " +
				"was never created; the route stays accepted but its " +
				"references do not resolve.",
			Root: "service/storefront/search-v3", Tier: "page",
			MaxMessages: 2,
			MustNotBlame: []string{
				"gateway.gateway.networking.k8s.io/storefront/edge",
				"service/storefront/search", "deployment/storefront/search"},
		},
		build: func(c *cluster) {
			c.list(c.node("w2", "zone-b"))
			routeBackendFleet(c, "search", "w2", 9090)
			c.list(routeGateway(c, "edge"))
			c.list(routeTo(c, "api", "edge", 3, []string{"search"}, ""))
			c.after(3 * time.Minute)
			c.update(routeTo(c, "api", "edge", 4, []string{"search-v3"},
				"search-v3"))
			c.after(6 * time.Minute)
		},
	}
}

// routeCanaryBackendMissing: a canary split adds a second backend,
// Service checkout-canary, to the checkout HTTPRoute, but the canary's
// Service was never applied. Part of the traffic now gets errors. The
// missing Service is the root, not the route or the stable backend.
func routeCanaryBackendMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "route-canary-backend-missing",
			Description: "An HTTPRoute canary split adds a backend Service " +
				"that does not exist next to a healthy one.",
			Root: "service/storefront/checkout-canary", Tier: "page",
			MaxMessages: 2,
			MustNotBlame: []string{
				"gateway.gateway.networking.k8s.io/storefront/edge",
				"service/storefront/checkout",
				"deployment/storefront/checkout"},
		},
		build: func(c *cluster) {
			c.list(c.node("w5", "zone-a"), c.node("w6", "zone-b"))
			routeBackendFleet(c, "checkout", "w5", 8443)
			c.list(routeGateway(c, "edge"))
			c.list(routeTo(c, "checkout", "edge", 7,
				[]string{"checkout"}, ""))
			c.after(2 * time.Minute)
			c.update(routeTo(c, "checkout", "edge", 8,
				[]string{"checkout", "checkout-canary"}, "checkout-canary"))
			c.after(5 * time.Minute)
		},
	}
}

// routeBackendFleet lists a healthy two-pod Deployment in storefront,
// running on node, with its Service and EndpointSlice on port.
func routeBackendFleet(c *cluster, name, node string, port int32) {
	w := c.deployment("storefront", name,
		"registry.example.com/"+name+":7", 2)
	admissionServes(w, port)
	c.list(w.objects())
	pods := []*corev1.Pod{w.pod(0, node), w.pod(1, node)}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "storefront", name, port),
		admissionSlice(c, "storefront", name, port, pods...))
}

// routeGateway is a storefront Gateway with one HTTPS listener.
func routeGateway(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "storefront", name, "gateway")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
		"spec": map[string]any{"gatewayClassName": "contour",
			"listeners": []any{map[string]any{"name": "https",
				"port": int64(443), "protocol": "HTTPS"}}},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	return u
}

// routeTo is a storefront HTTPRoute at generation, attached to gateway
// and sending traffic to backends. The Gateway accepts it; when
// missing is set, it also reports that backend not found.
func routeTo(c *cluster, name, gateway string, generation int64,
	backends []string, missing string) *unstructured.Unstructured {
	meta := clusterMeta(c, "storefront", name, "route")
	parent := map[string]any{"name": c.n(gateway), "sectionName": "https"}
	refs := make([]any, 0, len(backends))
	for _, backend := range backends {
		refs = append(refs, map[string]any{"name": c.n(backend),
			"port": int64(80), "weight": int64(100 / len(backends))})
	}
	conditions := []any{routeCondition(c, "Accepted", "True", "Accepted",
		"", generation)}
	if missing != "" {
		conditions = append(conditions, routeCondition(c, "ResolvedRefs",
			"False", "BackendNotFound",
			"Service \""+c.n(missing)+"\" not found", generation))
	}
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"spec": map[string]any{
			"parentRefs": []any{parent},
			"rules":      []any{map[string]any{"backendRefs": refs}},
		},
		"status": map[string]any{"parents": []any{map[string]any{
			"parentRef": parent, "controllerName": "contour",
			"conditions": conditions,
		}}},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(generation)
	return u
}

func routeCondition(c *cluster, kind, status, reason, message string,
	generation int64) map[string]any {
	return map[string]any{
		"type": kind, "status": status, "reason": reason,
		"message": message, "observedGeneration": generation,
		"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
	}
}

// routeMissingBackend: an HTTPRoute is edited to send traffic to
// Service checkout-v2, which was never deployed. The Gateway still
// accepts the route but reports ResolvedRefs False with BackendNotFound,
// and every request gets a 500. The missing Service is the root.
// Held out until 2026-10-01 as
// heldout-route-missing-backend; labelled since its miss was looked at
// (SCORECARD.md, held-out rotation).
func routeMissingBackend() scenario {
	return scenario{
		expect: expectation{
			Name: "route-missing-backend",
			Description: "An HTTPRoute names a backend Service that does " +
				"not exist; the route is accepted but its references do " +
				"not resolve.",
			Root: "service/shop/checkout-v2", Tier: "page", MaxMessages: 2,
			MustNotBlame: []string{
				"gateway.gateway.networking.k8s.io/shop/public",
				"service/shop/web", "deployment/shop/web"},
		},
		build: buildRouteMissingBackend,
	}
}

func buildRouteMissingBackend(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:2", 2)
	c.list(w.objects())
	pods := []*corev1.Pod{w.pod(0, "n1"), w.pod(1, "n1")}
	c.list(pods[0], pods[1])
	c.list(clusterService(c, "shop", "web", 8080),
		trafficSlice(c, "web", pods...))
	c.list(trafficGateway(c), trafficRoute(c, 1, "https", "True",
		"Accepted", ""))
	c.after(2 * time.Minute)
	route := trafficRoute(c, 2, "https", "True", "Accepted", "")
	rules := route.Object["spec"].(map[string]any)["rules"].([]any)
	backend := rules[0].(map[string]any)["backendRefs"].([]any)[0]
	backend.(map[string]any)["name"] = c.n("checkout-v2")
	parents := route.Object["status"].(map[string]any)["parents"].([]any)
	parent := parents[0].(map[string]any)
	parent["conditions"] = append(parent["conditions"].([]any),
		map[string]any{
			"type": "ResolvedRefs", "status": "False",
			"reason": "BackendNotFound",
			"message": "Service \"" + c.n("checkout-v2") +
				"\" not found",
			"observedGeneration": int64(2),
			"lastTransitionTime": c.now.UTC().Format(time.RFC3339),
		})
	c.update(route)
	c.after(5 * time.Minute)
}
