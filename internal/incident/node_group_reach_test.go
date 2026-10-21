package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node outage reaches the teams whose pods run on every node of the
// outage, also on a merged node whose own finding is already gone.
func TestGroupedOutageReachesThePodsOnItsMergedNodes(t *testing.T) {
	r := newRig(t, Config{})
	n2 := inventory.CoreID(kube.KindNode, "", "n2")
	pod := entity(kube.KindPod, "billing-0")
	deploy := entity(kube.KindDeployment, "billing")
	r.relate(pod, inventory.RunsOn, n2)
	r.relate(pod, inventory.OwnedBy, deploy)
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrOwner: inventory.Text("team-billing")})
	r.raise(at(0), lostNode("n1"), lostNode("n2"))
	wantAction(t, r.tick(at(DefaultPageSettle)), Announce, ReasonSettled)
	r.clear(at(time.Minute), lostNode("n2"))

	id := r.idOf(lostNode("n1").Entity)
	r.m.mu.Lock()
	p := r.m.incidents[id]
	owners, spaces := r.m.reachOf(p)
	r.m.mu.Unlock()

	assert.Equal(t, []string{"team-billing"}, owners)
	assert.Equal(t, []string{"shop"}, spaces)
}

// A forgotten outage takes the roots it merged with it.
func TestForgottenGroupedOutageLeavesNoGroupEntries(t *testing.T) {
	r := newRig(t, Config{})
	now := at(0)
	for round := range 3 {
		names := []string{"a", "b", "c"}
		var lost []string
		for _, n := range names {
			lost = append(lost, n+string(rune('0'+round)))
		}
		for _, n := range lost {
			r.raise(now, lostNode(n))
		}
		now = now.Add(DefaultPageSettle + time.Second)
		r.tick(now)
		now = now.Add(time.Minute)
		for _, n := range lost {
			r.clear(now, lostNode(n))
		}
		r.tick(now)
		now = now.Add(DefaultHold + time.Second)
		r.tick(now)
		now = now.Add(DefaultRemember + 24*time.Hour)
		r.tick(now)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	require.Empty(t, r.m.incidents, "every incident was forgotten")
	assert.Empty(t, r.m.groups)
}
