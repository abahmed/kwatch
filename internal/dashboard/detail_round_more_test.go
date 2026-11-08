package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestCandidatesListTheCauseAndItsRivalBestFirst(t *testing.T) {
	cause := &rootcause.CauseRecord{
		Root:  inventory.CoreID(kube.KindConfigMap, "shop", "a"),
		Score: 0.62,
		Proof: []rootcause.Proof{{Text: "began first", Supports: true}},
		Rival: &rootcause.CauseRecord{
			Root:  inventory.CoreID(kube.KindConfigMap, "shop", "b"),
			Score: 0.64,
			Proof: []rootcause.Proof{{Text: "pods differ"}}}}

	got := candidatesOf(cause, &RoundCheck{})

	require.Len(t, got, 2)
	assert.Equal(t, 64, got[0].Score)
	assert.Equal(t, "ruled out", got[0].State)
	assert.Equal(t, "kept", got[1].State)
	assert.False(t, got[0].Proofs[0].Supports)
	cause.Rival = nil
	assert.Nil(t, candidatesOf(cause, nil))
	assert.Nil(t, candidatesOf(nil, nil))
}

func TestCandidatesAreAClosecallWithoutACheck(t *testing.T) {
	cause := &rootcause.CauseRecord{Score: 0.6,
		Rival: &rootcause.CauseRecord{Score: 0.58}}
	got := candidatesOf(cause, nil)
	assert.Equal(t, "close", got[0].State)
	assert.Equal(t, "close", got[1].State)
}

func TestBlastPodsSplitReadersFromTheRest(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "orders-config")
	reader := inventory.CoreID(kube.KindPod, "shop", "orders-1")
	other := inventory.CoreID(kube.KindPod, "shop", "orders-2")
	for _, id := range []inventory.EntityID{cm, reader, other} {
		_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", At: roundAt, Entity: id})
		require.NoError(t, err)
	}
	_, err := m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: roundAt, Entity: reader, Relation: inventory.References,
		Targets: []inventory.EntityID{cm}})
	require.NoError(t, err)
	members := map[detection.Key]detection.Finding{}
	for _, pod := range []inventory.EntityID{reader, other} {
		f := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff"}
		members[f.Key()] = f
	}
	p := incident.Incident{Members: members,
		Cause: &rootcause.CauseRecord{Root: cm}}

	got := blastPodsOf(m, p, &RoundBlast{Count: 1, Total: 2})

	require.Len(t, got, 2)
	assert.Equal(t, RoundPod{Namespace: "shop", Name: "orders-1",
		Read: true}, got[0])
	assert.False(t, got[1].Read)
	assert.Nil(t, blastPodsOf(m, p, &RoundBlast{Count: 2, Total: 2}),
		"a list that contradicts the fraction is left out")
}

func TestLogRatesAreErrorsPerMinuteNowAndUsual(t *testing.T) {
	id := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	m := inventory.NewModel(inventory.Options{})
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: roundAt, Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrLogBaseline:     inventory.Number(30),
			kube.AttrLogBaselineSpan: inventory.Number(3600)}})
	require.NoError(t, err)

	now, usual := logRates(m, id, 120, 120)

	assert.Equal(t, 60.0, now)
	assert.Equal(t, 0.5, usual)
}
