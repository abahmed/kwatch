package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node named as what failing workloads share, while it looks healthy,
// is then really lost: the workloads rejoin its incident, and nothing is
// announced for them beside it.
func TestSuspectedNodeThatFailsTakesItsWorkloadsBack(t *testing.T) {
	r := newRig(t, Config{})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node n1 is what they share")
	r.cause(web.Entity, node, "node n1 is what they share")
	r.raise(at(0), api, web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, ReasonSettled)

	// The suspicion lapses and each pod goes its own way.
	delete(r.rule.causes, api.Entity)
	delete(r.rule.causes, web.Entity)
	r.raise(at(2*time.Minute), api, web)
	require.Len(t, r.m.Export(), 3, "two workload incidents are settling")

	r.raise(at(3*time.Minute), nodeDown())

	var got []Decision
	for now := 3 * time.Minute; now < 10*time.Minute; now +=
		10 * time.Second {
		got = append(got, r.tick(at(now))...)
	}
	for _, d := range got {
		assert.NotEqual(t, Announce, d.Action, "nothing new to announce")
	}
	held := r.of(node)
	for _, f := range []string{api.Entity.Name, web.Entity.Name} {
		found := false
		for key := range held.Members {
			found = found || key.Entity.Name == f
		}
		assert.True(t, found, "%s is back in the node incident", f)
	}
}

// Workloads that left a node incident stay out while the node is fine.
func TestSuspectedNodeThatStaysHealthyKeepsThemApart(t *testing.T) {
	r := newRig(t, Config{})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	api, web := podSig("api"), podSig("web")
	r.cause(api.Entity, node, "node n1 is what they share")
	r.cause(web.Entity, node, "node n1 is what they share")
	r.raise(at(0), api, web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, ReasonSettled)
	delete(r.rule.causes, api.Entity)
	delete(r.rule.causes, web.Entity)
	r.raise(at(2*time.Minute), api, web)

	r.raise(at(3*time.Minute), api, web)

	assert.Empty(t, r.of(node).Members)
}
