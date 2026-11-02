package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const gatewayAPI = "gateway.networking.k8s.io"

func addressRoute(hostnames []any, match map[string]any) map[string]any {
	rule := map[string]any{"backendRefs": []any{map[string]any{
		"name": "kwatch-dashboard", "port": int64(8061)}}}
	if match != nil {
		rule["matches"] = []any{match}
	}
	spec := map[string]any{
		"parentRefs": []any{map[string]any{"name": "edge"}},
		"rules":      []any{rule},
	}
	if hostnames != nil {
		spec["hostnames"] = hostnames
	}
	return map[string]any{
		"apiVersion": gatewayAPI + "/v1", "kind": "HTTPRoute",
		"metadata": map[string]any{"name": "dash", "namespace": "kwatch"},
		"spec":     spec,
	}
}

func addressGateway(listeners ...any) map[string]any {
	return map[string]any{
		"apiVersion": gatewayAPI + "/v1", "kind": "Gateway",
		"metadata": map[string]any{"name": "edge", "namespace": "kwatch"},
		"spec":     map[string]any{"listeners": listeners},
	}
}

// gatewayModel describes each object with its schema and puts it,
// with its relations, in a model.
func gatewayModel(t *testing.T, objects ...map[string]any) inventory.Reader {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	for _, object := range objects {
		u := &unstructured.Unstructured{Object: object}
		desc, ok := kube.NewUnstructuredSchema(gatewayAPI,
			u.GetKind()).Describe(u)
		require.True(t, ok)
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: desc.ID, Attributes: desc.Attributes,
		})
		require.NoError(t, err)
		for relation, targets := range desc.Relations {
			_, err := model.Apply(inventory.Observation{
				Kind: inventory.Related, Source: kube.ObservationSource,
				At: fixedTime(), Entity: desc.ID, Relation: relation,
				Targets: targets,
			})
			require.NoError(t, err)
		}
	}
	return model
}

func listenerOf(protocol, hostname string) map[string]any {
	l := map[string]any{"name": protocol, "protocol": protocol,
		"port": int64(443)}
	if hostname != "" {
		l["hostname"] = hostname
	}
	return l
}

// The scheme of a route's address is its Gateway listener's protocol.
func TestPublicAddressesOfHTTPRouteFollowGatewayListener(t *testing.T) {
	route := addressRoute([]any{"k.example.com"}, nil)
	cases := map[string]struct {
		listener map[string]any
		want     string
	}{
		"https": {listenerOf("HTTPS", "*.example.com"), "https"},
		"http":  {listenerOf("HTTP", ""), "http"},
		"other host": {listenerOf("HTTPS", "other.example.org"),
			""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := gatewayModel(t, addressGateway(tc.listener), route)
			got := kube.PublicAddresses(model, "kwatch")
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].Scheme)
			assert.Equal(t, "k.example.com", got[0].Host)
		})
	}
}

// A route without hostnames is reached on its listener's hostname; with
// no listener hostname known it names no address.
func TestPublicAddressesOfHTTPRouteTakeListenerHostname(t *testing.T) {
	route := addressRoute(nil, nil)
	model := gatewayModel(t,
		addressGateway(listenerOf("HTTPS", "dash.example.com")), route)
	got := kube.PublicAddresses(model, "kwatch")
	require.Len(t, got, 1)
	assert.Equal(t, "https", got[0].Scheme)
	assert.Equal(t, "dash.example.com", got[0].Host)

	assert.Empty(t, kube.PublicAddresses(gatewayModel(t, route), "kwatch"))
	wildcard := gatewayModel(t,
		addressGateway(listenerOf("HTTPS", "*.example.com")), route)
	assert.Empty(t, kube.PublicAddresses(wildcard, "kwatch"))
}

// An Exact path match is no prefix a link can open pages below, unless
// it is the root.
func TestPublicAddressesSkipExactPaths(t *testing.T) {
	exact := map[string]any{"path": map[string]any{
		"type": "Exact", "value": "/kw"}}
	root := map[string]any{"path": map[string]any{
		"type": "Exact", "value": "/"}}
	host := []any{"k.example.com"}
	assert.Empty(t, kube.PublicAddresses(
		gatewayModel(t, addressRoute(host, exact)), "kwatch"))
	got := kube.PublicAddresses(
		gatewayModel(t, addressRoute(host, root)), "kwatch")
	require.Len(t, got, 1)
	assert.Equal(t, "/", got[0].Path)

	pathType := networkingv1.PathTypeExact
	rule := addressRule("k.example.com", "/kw", "kwatch-dashboard",
		networkingv1.ServiceBackendPort{Number: 8061})
	rule.HTTP.Paths[0].PathType = &pathType
	assert.Empty(t, describedAddresses(t, addressIngress(nil, rule)))
}
