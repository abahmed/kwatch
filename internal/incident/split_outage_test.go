package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// splitRig announces a paged outage of a node pool whose two workloads
// then fail on two nodes of their own: web on n1, api on n2. The node
// incidents open settling. It returns the rig, the two findings and the
// ID of the pool incident.
func splitRig(t *testing.T) (*rig, detection.Finding, detection.Finding,
	string) {
	t.Helper()
	r := newRig(t, Config{})
	web, api := podSig("web"), podSig("api")
	pool := entity(kube.KindNodePool, "pool")
	r.cause(web.Entity, pool, "node pool pool is failing")
	r.cause(api.Entity, pool, "node pool pool is failing")
	r.raise(at(0), web, api)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	id := r.idOf(pool)
	r.m.RecordPaged(id, true)

	r.cause(web.Entity, entity(kube.KindNode, "n1"), "node n1 is down")
	r.cause(api.Entity, entity(kube.KindNode, "n2"), "node n2 is down")
	r.apply(at(2*time.Minute), detection.Changed, web, api)
	require.Empty(t, r.of(pool).Members)
	require.Equal(t, Settling, r.of(entity(kube.KindNode, "n1")).State)
	return r, web, api, id
}

// A paged pool outage that splits into outages of nodes still settling
// is not closed as "Moved" into them: a node incident that recovers
// before it is announced sends no message for the thread to point at.
// The pool incident goes on and resolves with a resolve of its own.
func TestSplitOutageSkipsANodeIncidentNeverAnnounced(t *testing.T) {
	r, web, api, id := splitRig(t)
	got := r.tick(at(2*time.Minute + time.Second))
	r.clear(at(2*time.Minute+20*time.Second), web, api)
	got = append(got, tickEvery(r, 2*time.Minute+20*time.Second,
		3*time.Hour)...)

	var resolve *Decision
	for i, d := range got {
		assert.NotEqual(t, ReasonSuperseded, d.Reason, "%+v", d)
		assert.NotEqual(t, Announce, d.Action, "no node incident spoke")
		if d.Action == Resolve && d.Incident.ID == id {
			resolve = &got[i]
		}
	}
	require.NotNil(t, resolve, "the pool incident resolves: %+v", got)
	assert.Empty(t, resolve.Incident.SupersededBy)
	assert.Empty(t, resolve.Incident.Moved.Into)
}

// The pool incident is moved into a node incident only once that one is
// announced, never before it.
func TestSplitOutageMovesAfterTheNodeIncidentIsAnnounced(t *testing.T) {
	r, _, _, id := splitRig(t)
	announced, moved := -1, -1
	got := tickEvery(r, 2*time.Minute, 10*time.Minute)
	for i, d := range got {
		switch {
		case d.Action == Announce && d.Incident.ID != id:
			if announced < 0 {
				announced = i
			}
		case d.Action == Resolve && d.Incident.ID == id:
			assert.Equal(t, ReasonSuperseded, d.Reason)
			assert.NotEmpty(t, d.Incident.Moved.Into)
			moved = i
		}
	}
	require.GreaterOrEqual(t, announced, 0, "%+v", got)
	require.GreaterOrEqual(t, moved, 0, "%+v", got)
	assert.Greater(t, moved, announced, "moved after the node spoke")
}
