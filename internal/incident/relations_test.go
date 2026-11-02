package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// relatedPair seeds a node incident whose chain goes on past its limit
// into the failures of a workload incident announced on its own.
func relatedPair(t *testing.T) (*Manager, *Incident, *Incident) {
	t.Helper()
	m := NewManager(Config{IDNonce: testNonce}, nil)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	api := entity(kube.KindDeployment, "api")
	pod := entity(kube.KindPod, "api-1")
	a := &Incident{ID: "a", Root: node, State: Open, Tier: Page,
		Headline: "Node n1 (prod-eu-1) is not ready.", Scope: ScopeIn,
		Cause: &rootcause.CauseRecord{Root: node,
			Hops:   []rootcause.Hop{{Entity: node}},
			Beyond: []rootcause.Hop{{Entity: pod}}}}
	b := &Incident{ID: "b", Root: api, State: Settling, Tier: Notify,
		Members: map[detection.Key]detection.Finding{
			{Entity: pod, Reason: "CrashLoop"}: {Entity: pod}}}
	m.incidents[a.ID], m.incidents[b.ID] = a, b
	return m, a, b
}

func TestIncidentRelationsFollowTheChainPastItsLimit(t *testing.T) {
	m, _, _ := relatedPair(t)

	b := m.Incident("b")
	require.Len(t, b.CausedBy, 1)
	assert.Equal(t, Related{ID: "a", Title: "Node n1 (prod-eu-1) is not ready.",
		Tier: Page, State: Open, Told: true}, b.CausedBy[0])
	assert.Empty(t, b.Causing)

	a := m.Incident("a")
	require.Len(t, a.Causing, 1)
	assert.Equal(t, "b", a.Causing[0].ID)
	assert.Equal(t, "deployment shop/api", a.Causing[0].Title,
		"an incident nobody heard of is named by its root")
	assert.False(t, a.Causing[0].Told)
}

func TestIncidentRelationsFollowABlamedRoot(t *testing.T) {
	m, a, b := relatedPair(t)
	a.Cause = nil
	b.Cause = &rootcause.CauseRecord{Root: a.Root}

	got := m.Incident("b")
	require.Len(t, got.CausedBy, 1)
	assert.Equal(t, "a", got.CausedBy[0].ID)
}

func TestIncidentRelationsSkipResolvedIncidents(t *testing.T) {
	m, a, _ := relatedPair(t)
	a.State = Resolved

	assert.Empty(t, m.Incident("b").CausedBy)
	assert.Empty(t, m.Incident("a").Causing)
}

func TestIncidentCauseToldNeedsAnAnnouncedCause(t *testing.T) {
	m, a, _ := relatedPair(t)
	_, ok := m.Incident("b").CauseTold()
	assert.True(t, ok)

	a.Scope = ScopeOut
	_, ok = m.Incident("b").CauseTold()
	assert.False(t, ok, "people never heard of an out-of-scope cause")
}

func TestIncidentRelationsReachDecisions(t *testing.T) {
	m, _, b := relatedPair(t)
	snap := b.Snapshot()
	m.relate(&snap)
	assert.Len(t, snap.CausedBy, 1)
	assert.Len(t, m.Incidents(), 2)
	for _, p := range m.Incidents() {
		if p.ID == "b" {
			assert.Len(t, p.CausedBy, 1)
		}
	}
}
