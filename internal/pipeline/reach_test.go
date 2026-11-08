package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var reachAt = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func reachModel(t *testing.T, ready, total float64) (*inventory.Model,
	inventory.EntityID, inventory.EntityID) {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	ingress := inventory.CoreID(kube.KindIngress, "shop", "storefront")
	service := inventory.CoreID(kube.KindService, "shop", "checkout")
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", "checkout-x")
	apply := func(o inventory.Observation) {
		o.Source, o.At = "test", reachAt
		_, err := m.Apply(o)
		require.NoError(t, err)
	}
	apply(inventory.Observation{Kind: inventory.Observed, Entity: service})
	apply(inventory.Observation{Kind: inventory.Observed, Entity: slice,
		Attributes: map[string]inventory.Value{
			kube.AttrEndpointsReady: inventory.Number(ready),
			kube.AttrEndpoints:      inventory.Number(total)}})
	apply(inventory.Observation{Kind: inventory.Related, Entity: slice,
		Relation: inventory.Backs,
		Targets:  []inventory.EntityID{service}})
	lines := "https\tshop.example.com\t/\tcheckout\t8080\n" +
		"https\tshop.example.com\t/checkout\tcheckout\t8080\n" +
		"https\tuser:pw@bad.example.com\t/\tcheckout\t8080\n" +
		"http\tother.example.com\t/\tcart\t8080"
	apply(inventory.Observation{Kind: inventory.Observed, Entity: ingress,
		Attributes: map[string]inventory.Value{
			kube.AttrPublicAddresses: inventory.Text(lines)}})
	return m, ingress, service
}

func TestReachOfNamesTheAddressesOfTheAffectedIngress(t *testing.T) {
	m, ingress, _ := reachModel(t, 0, 3)
	p := incident.Incident{Root: inventory.CoreID(
		kube.KindDeployment, "shop", "checkout"),
		Impact: []inventory.EntityID{ingress}}

	got := reachOf(m, p)

	require.Len(t, got, 2, "credentials in a host and other Services stay out")
	assert.Equal(t, "shop.example.com/checkout", got[0].Address(),
		"the more specific path first")
	assert.Equal(t, "shop.example.com", got[1].Address())
	assert.True(t, got[0].Down())
	assert.Equal(t, 3, got[0].Total)
}

func TestReachOfFindsAddressesThroughAnAffectedService(t *testing.T) {
	m, _, service := reachModel(t, 1, 3)
	p := incident.Incident{Root: service}
	got := reachOf(m, p)
	require.NotEmpty(t, got)
	assert.False(t, got[0].Down())
	assert.Equal(t, 1, got[0].Ready)
}

func TestReachOfIsEmptyWithoutAnIngressOrService(t *testing.T) {
	m, _, _ := reachModel(t, 0, 3)
	p := incident.Incident{Root: inventory.CoreID(
		kube.KindDeployment, "shop", "checkout")}
	assert.Empty(t, reachOf(m, p))
}

func TestReachOfCapsTheAddresses(t *testing.T) {
	m, ingress, _ := reachModel(t, 0, 3)
	lines := ""
	for i := range incident.MaxReach + 4 {
		lines += "https\th" + string(rune('a'+i)) +
			".example.com\t/\tcheckout\t8080\n"
	}
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: reachAt, Entity: ingress,
		Attributes: map[string]inventory.Value{
			kube.AttrPublicAddresses: inventory.Text(lines)}})
	require.NoError(t, err)
	got := reachOf(m, incident.Incident{Root: ingress})
	assert.Len(t, got, incident.MaxReach)
}
