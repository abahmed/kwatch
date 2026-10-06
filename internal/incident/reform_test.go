package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Members still failing when their incident resolves form an incident
// of their own, which settles and announces like any other.
func TestResolveLeavesNoFailingMemberUnowned(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 2, 0)
	failing := notReadySig(pod)
	node := sig(entity(kube.KindNode, "n1"), reasons.NodeNotReady,
		detection.Critical)
	r.raise(at(0), node)
	require.Len(t, r.tick(at(DefaultSettle)), 1)

	// The node's incident closes while a member that outlived the
	// cause, the pod, still fails.
	r.m.mu.Lock()
	p := r.m.lookup(node.Entity)
	delete(p.Members, node.Key())
	delete(r.m.byMember, node.Key())
	p.Members[failing.Key()] = failing
	r.m.byMember[failing.Key()] = p.ID
	p.State, p.Resolved = Resolved, at(time.Minute)
	resolvedID := p.ID
	r.m.reform(p, at(time.Minute))
	r.m.mu.Unlock()

	own := r.of(deploy)
	assert.Equal(t, Settling, own.State)
	assert.Contains(t, own.Members, failing.Key())
	assert.NotEqual(t, resolvedID, own.ID)

	ds := r.tick(at(time.Minute + DefaultSettle))
	wantAction(t, ds, Announce, "settled")
	assert.Equal(t, deploy, ds[0].Incident.Root)
}
