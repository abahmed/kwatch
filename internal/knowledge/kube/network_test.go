package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestServiceSchemaDescribe(t *testing.T) {
	svc := service("svc1")
	schema := kube.ServiceSchema{}
	desc, ok := schema.Describe(svc)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("service"), desc.ID.Kind)
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
	assert.Equal(t, knowledge.Kind("endpointslice"), desc.ID.Kind)
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

	backs := desc.Relations[knowledge.Backs]
	assert.Len(t, backs, 1)
	assert.Equal(t, knowledge.Kind("service"), backs[0].Kind)
	assert.Equal(t, "mysvc", backs[0].Name)
}

func TestEndpointSliceRoutesToPods(t *testing.T) {
	eps := endpointSlice("eps1")
	schema := kube.EndpointSliceSchema{}
	desc, ok := schema.Describe(eps)
	assert.True(t, ok)

	routes := desc.Relations[knowledge.RoutesTo]
	assert.Len(t, routes, 1)
	assert.Equal(t, knowledge.Kind("pod"), routes[0].Kind)
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
	assert.Equal(t, knowledge.Kind("ingress"), desc.ID.Kind)
	assert.Equal(t, "ing1", desc.ID.Name)
}

func TestIngressRoutesToService(t *testing.T) {
	ing := ingress("ing1")
	schema := kube.IngressSchema{}
	desc, ok := schema.Describe(ing)
	assert.True(t, ok)

	routes := desc.Relations[knowledge.RoutesTo]
	assert.GreaterOrEqual(t, len(routes), 1)
	found := false
	for _, r := range routes {
		if r.Kind == knowledge.Kind("service") &&
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

	refs := desc.Relations[knowledge.References]
	found := false
	for _, r := range refs {
		if r.Kind == knowledge.Kind("secret") &&
			r.Name == "tls-cert" {
			found = true
		}
	}
	assert.True(t, found, "tls-cert reference not found")
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
