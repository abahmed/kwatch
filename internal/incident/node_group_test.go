package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func lostNode(name string) detection.Finding {
	return sig(inventory.CoreID(kube.KindNode, "", name),
		reasons.NodeNotReady, detection.Critical)
}

// Nodes that fail in the same settle window are one outage: one incident,
// one announcement, one page.
func TestNodesLostTogetherShareOneIncident(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"), lostNode("n2"), lostNode("n3"))

	ds := r.tick(at(DefaultPageSettle))

	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, Page, ds[0].Incident.Tier)
	assert.True(t, ds[0].Incident.Grouped())
	assert.Len(t, ds[0].Incident.Merged, 2)
	assert.Len(t, ds[0].Incident.Members, 3)
	assert.Len(t, r.m.Export(), 1)
}

// A node lost after the outage was announced is news of its own: it is
// not folded into what people were already told.
func TestNodeLostAfterTheAnnouncementHasItsOwnIncident(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"))
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, ReasonSettled)

	r.raise(at(time.Minute), lostNode("n2"))

	assert.Len(t, r.m.Export(), 2)
	assert.False(t, r.of(inventory.CoreID(kube.KindNode, "", "n1")).Grouped())
}

// A pod placed under a node that joined the outage follows it, even when
// the solver names that node as its cause only now.
func TestPodOfAJoinedNodeJoinsTheOutage(t *testing.T) {
	r := newRig(t, Config{})
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	pod := podSig("api")
	r.cause(pod.Entity, n2, "node n2 is not ready")
	r.raise(at(0), lostNode("n1"), lostNode("n2"))
	r.raise(at(10*time.Second), pod)

	assert.Len(t, r.m.Export(), 1, "no incident of node n2's own")
	got := r.only()
	_, member := got.Members[pod.Key()]
	assert.True(t, member)
}

// A restart keeps the outage together: the roots it merged still lead
// to its incident.
func TestRestartKeepsTheNodeOutageTogether(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"), lostNode("n2"))
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, ReasonSettled)

	again := NewManager(Config{IDNonce: testNonce}, r.rule)
	again.Restore(r.m.Export(), at(time.Minute))

	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	require.NotNil(t, again.groupOf(n2))
	assert.Equal(t, r.only().ID, again.groupOf(n2).ID)
}
