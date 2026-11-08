package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var roundAt = time.Date(2026, 11, 1, 14, 2, 0, 0, time.UTC)

func hopOf(kind inventory.Kind, ns, name string, minute int) rootcause.Hop {
	return rootcause.Hop{Entity: inventory.CoreID(kind, ns, name),
		Began: roundAt.Add(time.Duration(minute) * time.Minute)}
}

func TestRoundOfCollectsWhatTheCauseRecordHolds(t *testing.T) {
	db := inventory.CoreID(kube.KindExternalEndpoint, "",
		"db.example.com:5432")
	p := incident.Incident{Root: db, Cause: &rootcause.CauseRecord{
		Root: db,
		Source: &rootcause.ConfigSource{ConfigMap: inventory.CoreID(
			kube.KindConfigMap, "shop", "api-config"), Key: "DB_HOST",
			Address: "db.example.com:5432", More: 2},
		Proof: []rootcause.Proof{
			{Code: rootcause.ProofCoOnset, Count: 2, Total: 4,
				Supports: true},
			{Code: rootcause.ProofBlastOverlap, Count: 9, Total: 10,
				Supports: true}},
		Hops: []rootcause.Hop{
			hopOf(kube.KindNode, "", "n3", 0),
			hopOf(kube.KindDeployment, "shop", "redis", 1),
			hopOf(kube.KindDeployment, "shop", "checkout", 1)},
		Rival: &rootcause.CauseRecord{Root: inventory.CoreID(
			"cluster-dns", "", "cluster-dns")}}}

	got := roundOf(nil, p, []string{"kwatch checked: DNS resolves db fine."})

	require.NotNil(t, got)
	assert.Equal(t, "api-config", got.Config.Name)
	assert.Equal(t, 2, got.Config.More)
	assert.Equal(t, 4, got.Together.Seconds)
	assert.Equal(t, RoundBlast{Count: 9, Total: 10, Verb: "read"},
		*got.Blast)
	require.Len(t, got.Chain, 3, "a chain of three steps")
	assert.Equal(t, "n3", got.Chain[0].Name)
	assert.Contains(t, got.Check.Text, "kwatch checked")
	assert.Equal(t, "cluster-dns//cluster-dns", got.Check.Rival)
}

func TestRoundOfLeavesOutAChainOfTwoAndAWeakShare(t *testing.T) {
	p := incident.Incident{Cause: &rootcause.CauseRecord{
		Root: inventory.CoreID(kube.KindConfigMap, "shop", "c"),
		Proof: []rootcause.Proof{{Code: rootcause.ProofBlastOverlap,
			Count: 2, Total: 10, Supports: true}},
		Hops: []rootcause.Hop{hopOf(kube.KindNode, "", "n3", 0),
			hopOf(kube.KindDeployment, "shop", "redis", 1)}}}
	assert.Nil(t, roundOf(nil, p, nil))
}

func TestRoundOfSaysWhetherAResolvedCauseIsStillThere(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n3")
	m := inventory.NewModel(inventory.Options{})
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: roundAt, Entity: node,
		Attributes: map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)}})
	require.NoError(t, err)
	p := incident.Incident{State: incident.Resolved,
		Cause: &rootcause.CauseRecord{Root: node}}
	assert.Equal(t, "still", roundOf(m, p, nil).CauseNow)
}

func TestRoundOfBurstShowsWhatKubernetesReports(t *testing.T) {
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	m := inventory.NewModel(inventory.Options{})
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: roundAt, Entity: orders,
		Attributes: map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(3),
			kube.AttrReadyReplicas: inventory.Number(3)}})
	require.NoError(t, err)
	f := detection.Finding{Entity: orders, Reason: "LogErrorBurst",
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceLogErrors, Value: "340"},
			{Label: detection.EvidenceLogWindow, Value: "300"},
			{Label: detection.EvidenceLogLine, Value: "pq: refused"},
			{Label: detection.EvidenceLogLineCount, Value: "312"},
			{Label: detection.EvidenceLogLineBefore, Value: "2"}}}
	p := incident.Incident{Root: orders,
		Members: map[detection.Key]detection.Finding{f.Key(): f}}

	got := roundOf(m, p, nil)

	require.NotNil(t, got.Burst)
	assert.Equal(t, RoundBurst{Errors: 340, WindowMin: 5,
		Line: "pq: refused", Count: 312, Before: 2, ReadyPods: 3,
		DesiredPods: 3}, *got.Burst)
}

func TestReachOfListsAddressesDownFirstAndDropsBadHosts(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	ingress := inventory.CoreID(kube.KindIngress, "shop", "storefront")
	service := inventory.CoreID(kube.KindService, "shop", "checkout")
	apply := func(o inventory.Observation) {
		o.Source, o.At = "test", roundAt
		_, err := m.Apply(o)
		require.NoError(t, err)
	}
	apply(inventory.Observation{Kind: inventory.Observed, Entity: service})
	lines := "https\tshop.example.com\t/\tcheckout\t8080\n" +
		"https\tshop.example.com\t/checkout\tcheckout\t8080\n" +
		"https\tu:p@bad.example.com\t/\tcheckout\t8080"
	apply(inventory.Observation{Kind: inventory.Observed, Entity: ingress,
		Attributes: map[string]inventory.Value{
			kube.AttrPublicAddresses: inventory.Text(lines)}})

	got := reachOf(m, TrafficRow{Kind: "ingress", Namespace: "shop",
		Name: "storefront"})

	require.Len(t, got, 2)
	assert.Equal(t, "shop.example.com/checkout", got[0].Address)
	assert.Equal(t, "shop.example.com", got[1].Address)
}
