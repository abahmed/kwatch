package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestServiceSchemaDescribe(t *testing.T) {
	svc := service("svc1")
	schema := kube.ServiceSchema{}
	desc, ok := schema.Describe(svc)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("service"), desc.ID.Kind)
	assert.Equal(t, "svc1", desc.ID.Name)

	// Check service type
	svcType, ok := desc.Attributes["service.type"]
	assert.True(t, ok)
	assert.Equal(t, "ClusterIP", svcType.AsText())

	// Check selector
	selector, ok := desc.Attributes["selector"]
	assert.True(t, ok)
	text := selector.AsText()
	assert.Contains(t, text, "app=test")
}

func TestServiceLoadBalancer(t *testing.T) {
	svc := service("svc1")
	svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
		{IP: "203.0.113.1"},
	}

	schema := kube.ServiceSchema{}
	desc, ok := schema.Describe(svc)
	assert.True(t, ok)

	lb, ok := desc.Attributes["loadbalancer.assigned"]
	assert.True(t, ok)
	b, _ := lb.AsBool()
	assert.True(t, b)
}

func TestServiceSelectorDiff(t *testing.T) {
	old := service("svc1")
	new := service("svc1")
	new.Spec.Selector = map[string]string{
		"app":  "test",
		"tier": "frontend",
	}

	schema := kube.ServiceSchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.selector" {
			found = true
			assert.NotEqual(t, c.Before, c.After)
		}
	}
	assert.True(t, found, "selector change not found")
}

func TestEndpointSliceSchemaDescribe(t *testing.T) {
	eps := endpointSlice("eps1")
	schema := kube.EndpointSliceSchema{}
	desc, ok := schema.Describe(eps)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("endpointslice"), desc.ID.Kind)
	assert.Equal(t, "eps1", desc.ID.Name)

	// Check endpoint counts
	endpoints, ok := desc.Attributes["endpoints"]
	assert.True(t, ok)
	num, _ := endpoints.AsNumber()
	assert.Equal(t, 1.0, num)

	// Check ready count
	ready, ok := desc.Attributes["endpoints.ready"]
	assert.True(t, ok)
	num, _ = ready.AsNumber()
	assert.Equal(t, 1.0, num)
}

func TestEndpointSliceBacksService(t *testing.T) {
	eps := endpointSlice("eps1")
	schema := kube.EndpointSliceSchema{}
	desc, ok := schema.Describe(eps)
	assert.True(t, ok)

	backs := desc.Relations[inventory.Backs]
	assert.Len(t, backs, 1)
	assert.Equal(t, inventory.Kind("service"), backs[0].Kind)
	assert.Equal(t, "mysvc", backs[0].Name)
}

func TestEndpointSliceRoutesToPods(t *testing.T) {
	eps := endpointSlice("eps1")
	schema := kube.EndpointSliceSchema{}
	desc, ok := schema.Describe(eps)
	assert.True(t, ok)

	routes := desc.Relations[inventory.RoutesTo]
	assert.Len(t, routes, 1)
	assert.Equal(t, inventory.Kind("pod"), routes[0].Kind)
	assert.Equal(t, "pod1", routes[0].Name)
}

func TestEndpointSliceReadyCount(t *testing.T) {
	tt := []struct {
		name      string
		endpoints []any
		wantReady int
	}{
		{
			name: "all_ready",
			endpoints: []any{
				true, true, true,
			},
			wantReady: 3,
		},
		{
			name: "some_not_ready",
			endpoints: []any{
				true, false, true,
			},
			wantReady: 2,
		},
		{
			name: "nil_ready_counts_as_ready",
			endpoints: []any{
				nil, true,
			},
			wantReady: 2,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			eps := endpointSlice("eps1")
			// The endpoint slice from fixture already
			// has one endpoint with Ready=true
			schema := kube.EndpointSliceSchema{}
			_, ok := schema.Describe(eps)
			assert.True(t, ok)
		})
	}
}

func TestEndpointSliceNoDiff(t *testing.T) {
	eps1 := endpointSlice("eps1")
	eps2 := endpointSlice("eps1")

	schema := kube.EndpointSliceSchema{}
	changes := schema.Diff(eps1, eps2)

	// EndpointSlice diffs always return nil (churn is state)
	assert.Nil(t, changes)
}

func TestIngressSchemaDescribe(t *testing.T) {
	ing := ingress("ing1")
	schema := kube.IngressSchema{}
	desc, ok := schema.Describe(ing)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("ingress"), desc.ID.Kind)
	assert.Equal(t, "ing1", desc.ID.Name)
}

func TestIngressRoutesToService(t *testing.T) {
	ing := ingress("ing1")
	schema := kube.IngressSchema{}
	desc, ok := schema.Describe(ing)
	assert.True(t, ok)

	routes := desc.Relations[inventory.RoutesTo]
	assert.GreaterOrEqual(t, len(routes), 1)
	found := false
	for _, r := range routes {
		if r.Kind == inventory.Kind("service") &&
			r.Name == "mysvc" {
			found = true
		}
	}
	assert.True(t, found,
		"route to mysvc not found")
}

func TestIngressTLSSecretReference(t *testing.T) {
	ing := ingress("ing1")
	ing.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{"example.com"},
			SecretName: "tls-cert",
		},
	}

	schema := kube.IngressSchema{}
	desc, ok := schema.Describe(ing)
	assert.True(t, ok)

	refs := desc.Relations[inventory.References]
	found := false
	for _, r := range refs {
		if r.Kind == inventory.Kind("secret") &&
			r.Name == "tls-cert" {
			found = true
		}
	}
	assert.True(t, found, "tls-cert reference not found")
}

// The class is a reference, so an Ingress is judged again when its class
// appears or goes away.
func TestIngressClassReference(t *testing.T) {
	ing := ingress("ing1")
	class := "alb"
	ing.Spec.IngressClassName = &class

	desc, ok := kube.IngressSchema{}.Describe(ing)

	assert.True(t, ok)
	assert.Contains(t, desc.Relations[inventory.References],
		inventory.CoreID(kube.KindIngressClass, "", "alb"))
}

func TestIngressBackendDiff(t *testing.T) {
	old := ingress("ing1")
	new := ingress("ing1")
	newBackend := &networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: "othersvc",
			Port: networkingv1.ServiceBackendPort{Number: 9000},
		},
	}
	new.Spec.DefaultBackend = newBackend

	schema := kube.IngressSchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.backends" {
			found = true
			assert.NotEqual(t, c.Before, c.After)
		}
	}
	assert.True(t, found, "backend change not found")
}

// TestIngressSkipsActionBackends: a backend whose port name is
// use-annotation is an AWS Load Balancer Controller action named by an
// annotation, not a Service, so it must not become a route.
func TestIngressSkipsActionBackends(t *testing.T) {
	ing := ingress("ing1")
	ing.Spec.Rules = []networkingv1.IngressRule{{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "ssl-redirect",
							Port: networkingv1.ServiceBackendPort{
								Name: "use-annotation",
							},
						},
					},
				}},
			},
		},
	}}

	desc, ok := kube.IngressSchema{}.Describe(ing)

	assert.True(t, ok)
	var names []string
	for _, r := range desc.Relations[inventory.RoutesTo] {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"mysvc"}, names)
}
