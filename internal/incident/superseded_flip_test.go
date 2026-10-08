package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// supersededRig announces a node incident holding api and a web
// incident, then revises web onto the node: the web incident is closed
// as superseded and the node incident holds both failures.
func supersededRig(t *testing.T) (*rig, detection.Finding, string) {
	t.Helper()
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node down")
	r.raise(at(0), api, web)
	require.Len(t, r.tick(at(DefaultSettle)), 2)
	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	wantAction(t, r.tick(at(2*time.Minute+time.Second)), Resolve,
		ReasonSuperseded)
	return r, web, r.idOf(node)
}

// A crash loop makes the reasoning blame its object, then the node,
// then its object again. When the failure goes back to the root of the
// incident that was superseded, it stays with the incident that took
// over: no new incident, no new page.
func TestFailureBackToSupersededRootStaysWithTheTakeover(t *testing.T) {
	r, web, nodeID := supersededRig(t)

	delete(r.rule.causes, web.Entity)
	r.apply(at(3*time.Minute), detection.Changed, web)
	wantNone(t, r.tick(at(3*time.Minute+DefaultSettle)))

	assert.Len(t, r.m.Export(), 2, "no third incident is opened")
	assert.Equal(t, nodeID, r.m.byMember[web.Key()])
}

// Long after the takeover, the failure of the old root is news again.
func TestFailureBackAfterTheRepageWindowIsNews(t *testing.T) {
	r, web, nodeID := supersededRig(t)

	delete(r.rule.causes, web.Entity)
	r.apply(at(3*time.Minute+RepageWindow), detection.Changed, web)

	assert.NotEqual(t, nodeID, r.m.byMember[web.Key()])
}

// An incident is not superseded into one that has already resolved.
func TestSupersedeIntoResolvedIncidentIsCancelled(t *testing.T) {
	r := newRig(t, Config{})
	node := entity(kube.KindNode, "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node down")
	r.raise(at(0), api, web)
	require.Len(t, r.tick(at(DefaultSettle)), 2)
	r.cause(web.Entity, node, "node down")
	r.apply(at(2*time.Minute), detection.Changed, web)
	r.m.mu.Lock()
	r.m.lookup(node).State = Resolved
	r.m.mu.Unlock()

	for _, d := range r.tick(at(2*time.Minute + time.Second)) {
		assert.NotEqual(t, ReasonSuperseded, d.Reason)
	}
	assert.Empty(t, r.of(web.Entity).SupersededBy)
}
