package inventory

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gatewayAPI = "gateway.networking.k8s.io"

func TestEntityIDStringKeepsBuiltinKeysStable(t *testing.T) {
	tests := []struct {
		name string
		id   EntityID
		want string
	}{
		{"core pod", CoreID("pod", "shop", "api-1"), "pod/shop/api-1"},
		{"cluster-scoped", CoreID("node", "", "n1"), "node//n1"},
		{"virtual", CoreID("cluster-dns", "", "cluster-dns"),
			"cluster-dns//cluster-dns"},
		{"grouped", NewEntityID(gatewayAPI, "gateway", "infra", "gw"),
			"gateway.gateway.networking.k8s.io/infra/gw"},
		{"grouped cluster-scoped",
			NewEntityID("example.com", "widget", "", "w"),
			"widget.example.com//w"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.id.String())
			parsed, ok := ParseEntityID(tt.want)
			require.True(t, ok)
			assert.Equal(t, tt.id, parsed)
		})
	}
}

func TestParseEntityIDRejectsEmptyGroupOrKind(t *testing.T) {
	for _, key := range []string{"pod./ns/x", ".example.com/ns/x"} {
		_, ok := ParseEntityID(key)
		assert.False(t, ok, key)
	}
}

func TestEntityIDJSONOmitsBuiltinGroup(t *testing.T) {
	core, err := json.Marshal(CoreID("pod", "ns", "p"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"Kind":"pod","Namespace":"ns","Name":"p"}`,
		string(core))

	var legacy EntityID
	require.NoError(t, json.Unmarshal(core, &legacy))
	assert.Equal(t, CoreID("pod", "ns", "p"), legacy)

	grouped := NewEntityID(gatewayAPI, "gateway", "ns", "gw")
	data, err := json.Marshal(grouped)
	require.NoError(t, err)
	var back EntityID
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, grouped, back)
}

func TestObservationJSONDecodesLogsWithoutGroup(t *testing.T) {
	var o Observation
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"gone",`+
		`"entity":{"kind":"gateway","namespace":"ns","name":"gw"}}`), &o))
	assert.Equal(t, CoreID("gateway", "ns", "gw"), o.Entity)

	grouped := Observation{
		Kind: Related, Entity: NewEntityID(gatewayAPI, "httproute", "ns", "r"),
		Relation: References,
		Targets:  []EntityID{NewEntityID(gatewayAPI, "gateway", "ns", "gw")},
	}
	data, err := json.Marshal(grouped)
	require.NoError(t, err)
	var back Observation
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, grouped.Entity, back.Entity)
	assert.Equal(t, grouped.Targets, back.Targets)
}

func TestModelKeepsSameKindInDifferentGroupsApart(t *testing.T) {
	m := NewModel(Options{})
	at := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	gatewayAPIGateway := NewEntityID(gatewayAPI, "gateway", "infra", "gw")
	istioGateway := NewEntityID("networking.istio.io", "gateway", "infra",
		"gw")
	for _, id := range []EntityID{gatewayAPIGateway, istioGateway} {
		_, err := m.Apply(Observation{
			Kind: Observed, Source: "k8s", At: at, Entity: id,
			Attributes: map[string]Value{"group": Text(id.Group)},
		})
		require.NoError(t, err)
	}
	assert.Equal(t, []EntityID{gatewayAPIGateway, istioGateway},
		m.Entities("gateway"))
	entity, ok := m.Entity(istioGateway)
	require.True(t, ok)
	assert.Equal(t, "networking.istio.io",
		entity.Attributes["group"].Value.AsText())

	_, err := m.Apply(Observation{
		Kind: Gone, Source: "k8s", At: at.Add(time.Second),
		Entity: gatewayAPIGateway,
	})
	require.NoError(t, err)
	assert.False(t, m.Exists(gatewayAPIGateway))
	assert.True(t, m.Exists(istioGateway))
	assert.Equal(t, []EntityID{istioGateway}, m.Entities("gateway"))
}
