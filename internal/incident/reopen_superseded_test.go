package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An incident resolves, fails again and, before its "failing again"
// update is out, loses its failure to another announced incident. The
// last word of its thread is its resolve; a second resolve ("moved")
// right after it, with nothing in between, is not sent.
func TestReopenedThenSupersededIncidentDoesNotResolveTwice(t *testing.T) {
	r := newRig(t, Config{})
	node, api := entity(kube.KindNode, "n1"), entity(kube.KindPod, "api")
	web := podSig("web")
	r.cause(api, node, "node n1 is down")
	r.raise(at(0), web, podSig("api"))
	tickEvery(r, 0, 2*time.Minute)
	id := r.idOf(web.Entity)
	require.NotEqual(t, id, r.idOf(node))

	r.clear(at(5*time.Minute), web)
	var actions []Action
	collect := func(from, to time.Duration) {
		for _, d := range tickEvery(r, from, to) {
			if d.Incident.ID == id {
				actions = append(actions, d.Action)
			}
		}
	}
	collect(5*time.Minute, 20*time.Minute)
	require.Equal(t, []Action{Resolve}, actions)

	back := 21 * time.Minute
	r.raise(at(back), web)
	collect(back, back+10*time.Second)
	r.cause(web.Entity, node, "node n1 is down")
	r.raise(at(back+15*time.Second), web)
	collect(back+20*time.Second, back+10*time.Minute)

	assert.Equal(t, r.idOf(node), r.ofMember(web).ID, "moved to the node")
	assert.Equal(t, []Action{Resolve}, actions, "no second resolve")
}
