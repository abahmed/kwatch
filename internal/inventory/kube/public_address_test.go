package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func addressIngress(
	tls []networkingv1.IngressTLS, rules ...networkingv1.IngressRule,
) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "kwatch"},
		Spec:       networkingv1.IngressSpec{TLS: tls, Rules: rules},
	}
}

func addressRule(
	host, path, svc string, port networkingv1.ServiceBackendPort,
) networkingv1.IngressRule {
	return networkingv1.IngressRule{Host: host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{Path: path,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: svc, Port: port}}}}}}}
}

func describedAddresses(t *testing.T, obj any) []kube.PublicAddress {
	t.Helper()
	var desc kube.Description
	var ok bool
	switch o := obj.(type) {
	case *networkingv1.Ingress:
		desc, ok = kube.IngressSchema{}.Describe(o)
	case *unstructured.Unstructured:
		desc, ok = kube.NewUnstructuredSchema("gateway.networking.k8s.io",
			"HTTPRoute").Describe(o)
	}
	require.True(t, ok)
	model := inventory.NewModel(inventory.Options{})
	_, err := model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: kube.ObservationSource,
		At: fixedTime(), Entity: desc.ID, Attributes: desc.Attributes,
	})
	require.NoError(t, err)
	return kube.PublicAddresses(model, "kwatch")
}

func TestPublicAddressesOfIngressFollowTLS(t *testing.T) {
	named := networkingv1.ServiceBackendPort{Name: "dashboard"}
	ing := addressIngress(
		[]networkingv1.IngressTLS{{Hosts: []string{"*.example.com"}}},
		addressRule("kwatch.example.com", "", "kwatch-dashboard", named),
		addressRule("plain.example.org", "/ops", "kwatch-dashboard",
			networkingv1.ServiceBackendPort{Number: 8061}),
		addressRule("", "/", "kwatch-dashboard", named),
		addressRule("lb.example.com", "/", "kwatch-dashboard",
			networkingv1.ServiceBackendPort{Name: "use-annotation"}),
	)
	got := describedAddresses(t, ing)
	require.Len(t, got, 2)
	id := inventory.CoreID(kube.KindIngress, "kwatch", "web")
	assert.ElementsMatch(t, []kube.PublicAddress{
		{Scheme: "https", Host: "kwatch.example.com", Path: "/",
			Service: "kwatch-dashboard", Port: "dashboard", Source: id},
		{Scheme: "http", Host: "plain.example.org", Path: "/ops",
			Service: "kwatch-dashboard", Port: "8061", Source: id},
	}, got)
}

func TestPublicAddressesTLSWithoutHostsCoversEveryHost(t *testing.T) {
	ing := addressIngress([]networkingv1.IngressTLS{{SecretName: "tls"}},
		addressRule("a.example.com", "/", "svc",
			networkingv1.ServiceBackendPort{Number: 80}))
	got := describedAddresses(t, ing)
	require.Len(t, got, 1)
	assert.Equal(t, "https", got[0].Scheme)
}

func TestPublicAddressesOfHTTPRoute(t *testing.T) {
	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]any{
			"name": "dash", "namespace": "kwatch", "generation": int64(1)},
		"spec": map[string]any{
			"hostnames": []any{"kwatch.example.com", "*.example.com"},
			"rules": []any{map[string]any{
				"matches": []any{map[string]any{"path": map[string]any{
					"type": "PathPrefix", "value": "/kw"}}},
				"backendRefs": []any{
					map[string]any{"name": "kwatch-dashboard",
						"port": int64(8061)},
					map[string]any{"name": "other", "namespace": "else",
						"port": int64(80)},
				},
			}},
		},
	}}
	got := describedAddresses(t, route)
	require.Len(t, got, 1)
	// Without its Gateway in the model the scheme is unknown.
	assert.Equal(t, kube.PublicAddress{Scheme: "",
		Host: "kwatch.example.com", Path: "/kw",
		Service: "kwatch-dashboard", Port: "8061",
		Source: inventory.NewEntityID("gateway.networking.k8s.io",
			kube.KindHTTPRoute, "kwatch", "dash")}, got[0])
}

func TestPublicAddressesNoneWithoutRoutes(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	assert.Empty(t, kube.PublicAddresses(model, "kwatch"))
}
