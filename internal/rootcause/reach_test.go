package rootcause

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// chain is ingress → service → slice → pod → node, as relations point:
// each entity relates to what it depends on or is backed by.
func reachChain(t *testing.T) (*graph, []inventory.EntityID) {
	h := newGraph(t)
	ing := eid(kube.KindIngress, "ns", "web")
	svc := eid(kube.KindService, "ns", "web")
	slice := eid(kube.KindEndpointSlice, "ns", "web-x")
	pod := podID("web-0")
	node := eid(kube.KindNode, "", "n1")
	h.relate(ing, inventory.RoutesTo, svc)
	h.relate(slice, inventory.Backs, svc)
	h.relate(slice, inventory.RoutesTo, pod)
	h.relate(pod, inventory.RunsOn, node)
	return h, []inventory.EntityID{ing, svc, slice, pod, node}
}

func TestReachFollowsDependenciesUpstream(t *testing.T) {
	h, c := reachChain(t)

	got := Reach(h.model, c[0], inventory.Outgoing, 3)

	assert.Equal(t, []inventory.EntityID{c[1]}, ids(got))
	assert.Equal(t, inventory.RoutesTo, got[0].Via)
	assert.Equal(t, []inventory.EntityID{c[0], c[1]}, got[0].Path)
}

func TestReachFollowsDependentsDownstreamWithinHops(t *testing.T) {
	h, c := reachChain(t)

	got := Reach(h.model, c[4], inventory.Incoming, 2)

	assert.Equal(t, []inventory.EntityID{c[3], c[2]}, ids(got))
	assert.Equal(t, 2, got[1].Hops)
}

func TestReachFiltersRelationTypes(t *testing.T) {
	h, c := reachChain(t)

	got := Reach(h.model, c[2], inventory.Outgoing, 3, inventory.Backs)

	assert.Equal(t, []inventory.EntityID{c[1]}, ids(got))
}

func TestReachVisitsEachEntityOnce(t *testing.T) {
	h := newGraph(t)
	a, b, c := podID("a"), podID("b"), podID("c")
	h.relate(a, inventory.Calls, b)
	h.relate(a, inventory.References, c)
	h.relate(b, inventory.Calls, c)
	h.relate(c, inventory.Calls, a)

	got := Reach(h.model, a, inventory.Outgoing, 5)

	assert.Equal(t, []inventory.EntityID{b, c}, ids(got))
	assert.Equal(t, 1, got[1].Hops, "the shortest path wins")
}

func ids(reached []Reached) []inventory.EntityID {
	out := make([]inventory.EntityID, len(reached))
	for i, r := range reached {
		out[i] = r.ID
	}
	return out
}

// countingReader counts the entities whose relations are read.
type countingReader struct {
	inventory.Reader
	reads int
}

func (c *countingReader) Relations(
	id inventory.EntityID, dir inventory.Direction,
) []inventory.Relation {
	c.reads++
	return c.Reader.Relations(id, dir)
}

// Reach stops expanding once maxReach is met: a hub with many branches
// is not walked to the end of the level only to drop most of it.
func TestReachStopsExpandingAtTheCap(t *testing.T) {
	h := newGraph(t)
	hub := podID("hub")
	var mids []inventory.EntityID
	for i := 0; i < 10; i++ {
		mid := podID(fmt.Sprintf("mid-%d", i))
		mids = append(mids, mid)
		var leaves []inventory.EntityID
		for j := 0; j < 100; j++ {
			leaves = append(leaves, podID(fmt.Sprintf("leaf-%d-%d", i, j)))
		}
		h.relateAll(mid, leaves)
	}
	h.relateAll(hub, mids)
	reader := &countingReader{Reader: h.model}

	got := Reach(reader, hub, inventory.Outgoing, 2)

	assert.Len(t, got, maxReach)
	assert.LessOrEqual(t, reader.reads, 5)
}

func (g *graph) relateAll(from inventory.EntityID, to []inventory.EntityID) {
	g.t.Helper()
	if _, err := g.model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", Entity: from,
		Relation: inventory.Calls, Targets: to,
	}); err != nil {
		g.t.Fatal(err)
	}
}
