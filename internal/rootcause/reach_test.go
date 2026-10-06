package rootcause

import (
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
