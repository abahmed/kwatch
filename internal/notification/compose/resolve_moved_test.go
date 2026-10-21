package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var (
	movedNode = inventory.CoreID(kube.KindNode, "", "n1")
	movedCart = inventory.CoreID(kube.KindDeployment, "shop", "cart")
	movedPod  = inventory.CoreID(kube.KindPod, "shop", "cart-1-a")
)

// movedIncident is an incident announced for node n1 that a cause
// revision moved to deployment cart: the node came back after four
// minutes, cart kept failing and was healthy again at 10:14.
func movedIncident(t *testing.T, tier incident.Tier) incident.Incident {
	t.Helper()
	min := func(m int) time.Time {
		return time.Date(2026, 9, 29, 10, m, 0, 0, time.UTC)
	}
	about := func(id inventory.EntityID) *inventory.EntityID { return &id }
	m := incident.NewManager(incident.Config{}, nil)
	m.Restore([]incident.Record{{ID: "inc-1", Root: movedCart, Tier: tier,
		State: incident.Resolved, Opened: min(2), Announced: min(3),
		RecoveringSince: min(14), Resolved: min(17), Revision: 3,
		Subject: movedNode, FormerRoot: movedNode, RerootedAt: min(6),
		Timeline: []incident.Event{
			{At: min(2), Entity: about(movedNode),
				Text: "Node is NotReady (node n1)"},
			{At: min(4), Entity: about(movedPod),
				Text: "Pod has not been ready for 3m (pod shop/cart-1-a)"},
			{At: min(6), Entity: about(movedNode),
				Text: incident.RecoveredPrefix + "Node is NotReady (node n1)"},
			{At: min(6),
				Text: "cause revised: now explained by deployment shop/cart"},
			{At: min(14), Entity: about(movedPod),
				Text: incident.RecoveredPrefix +
					"Pod has not been ready for 3m (pod shop/cart-1-a)"},
			{At: min(17), Text: "resolved: healthy for 3m0s"},
		}}}, time.Time{})
	all := m.Incidents()
	require.Len(t, all, 1)
	return all[0]
}

// The resolve of an incident that moved from a node to a workload says
// the workload is healthy again, for as long as the workload failed: not
// that the node it was announced for is ready, for as long as the whole
// incident lasted.
func TestResolveOfAMovedIncidentDescribesTheCurrentRoot(t *testing.T) {
	p := movedIncident(t, incident.Page)
	require.Equal(t, movedNode, p.FormerRoot())

	_, got := resolveNote(caseFacts{p: p, now: p.Resolved})

	require.Len(t, got, 2)
	assert.Equal(t, "cart in shop is healthy again.", got[0].text)
	assert.Equal(t, "It was failing for ten minutes.", got[1].text)
}

// A page for a node does not make the workload it moved to "down".
func TestResolveOfAMovedIncidentDoesNotSayDown(t *testing.T) {
	p := movedIncident(t, incident.Page)

	_, got := resolveNote(caseFacts{p: p, now: p.Resolved})

	assert.NotContains(t, got[1].text, "down")
}

// Without a move the resolve keeps naming the announced subject.
func TestResolveKeepsTheAnnouncedSubjectWhenNothingMoved(t *testing.T) {
	p := movedIncident(t, incident.Notify)
	m := incident.NewManager(incident.Config{}, nil)
	m.Restore([]incident.Record{{ID: "inc-2", Root: movedCart,
		State: incident.Resolved, Opened: p.Opened, Resolved: p.Resolved,
		RecoveringSince: p.RecoveringSince, Subject: movedNode}},
		time.Time{})
	unmoved := m.Incidents()[0]

	_, got := resolveNote(caseFacts{p: unmoved, now: unmoved.Resolved})

	assert.True(t, strings.HasPrefix(got[0].text, "Node n1 is"), got[0].text)
}
