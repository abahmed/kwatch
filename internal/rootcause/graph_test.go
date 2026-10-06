package rootcause

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// graph is a model with helpers to relate entities.
type graph struct {
	t     *testing.T
	model *inventory.Model
}

func newGraph(t *testing.T) *graph {
	return &graph{t: t, model: inventory.NewModel(inventory.Options{})}
}

func (g *graph) relate(
	from inventory.EntityID, relation inventory.RelationType,
	to inventory.EntityID,
) {
	g.t.Helper()
	if _, err := g.model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", Entity: from,
		Relation: relation, Targets: []inventory.EntityID{to},
	}); err != nil {
		g.t.Fatal(err)
	}
}

func (g *graph) observe(id inventory.EntityID) {
	g.t.Helper()
	if _, err := g.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", Entity: id,
	}); err != nil {
		g.t.Fatal(err)
	}
}

func eid(kind inventory.Kind, ns, name string) inventory.EntityID {
	return inventory.CoreID(kind, ns, name)
}

func podID(name string) inventory.EntityID {
	return eid(kube.KindPod, "ns", name)
}

func (g *graph) relateOwned(pod, owner inventory.EntityID) {
	g.relate(pod, inventory.OwnedBy, owner)
}

func TestPodOfResolvesPodAndContainer(t *testing.T) {
	h := newGraph(t)
	p := podID("p1")
	c := eid(kube.KindContainer, "ns", "app")
	h.observe(p)
	h.relate(c, inventory.PartOf, p)

	if got, ok := PodOf(h.model, p); !ok || got != p {
		t.Errorf("pod: got %v %v", got, ok)
	}
	if got, ok := PodOf(h.model, c); !ok || got != p {
		t.Errorf("container: got %v %v", got, ok)
	}
}

func TestPodOfRejectsOrphanContainerAndOtherKinds(t *testing.T) {
	h := newGraph(t)
	orphan := eid(kube.KindContainer, "ns", "lonely")
	if _, ok := PodOf(h.model, orphan); ok {
		t.Error("orphan container has no pod")
	}
	if _, ok := PodOf(h.model, eid(kube.KindNode, "", "n1")); ok {
		t.Error("node is not a pod")
	}
}

func TestOwnerChainWalksToTopController(t *testing.T) {
	h := newGraph(t)
	p, rs, d := podID("p1"), eid(kube.KindReplicaSet, "ns", "rs"),
		eid(kube.KindDeployment, "ns", "d")
	h.relateOwned(p, rs)
	h.relate(rs, inventory.OwnedBy, d)

	chain := OwnerChain(h.model, p)
	if len(chain) != 2 || chain[0] != rs || chain[1] != d {
		t.Fatalf("chain = %v", chain)
	}
	if TopOwner(h.model, p) != d {
		t.Error("top owner should be the deployment")
	}
}

func TestOwnerChainExcludesStaticPodNodeOwner(t *testing.T) {
	h := newGraph(t)
	p, n := podID("static"), eid(kube.KindNode, "", "n1")
	h.relateOwned(p, n)

	if chain := OwnerChain(h.model, p); len(chain) != 0 {
		t.Errorf("node must not be an owner, got %v", chain)
	}
	if TopOwner(h.model, p) != p {
		t.Error("static pod is its own top owner")
	}
}

func TestOwnerChainStopsOnCycle(t *testing.T) {
	h := newGraph(t)
	a, b := eid(kube.KindReplicaSet, "ns", "a"),
		eid(kube.KindReplicaSet, "ns", "b")
	h.relate(a, inventory.OwnedBy, b)
	h.relate(b, inventory.OwnedBy, a)

	if chain := OwnerChain(h.model, a); len(chain) != 1 || chain[0] != b {
		t.Errorf("cycle chain = %v", chain)
	}
}

func TestOwnedPodsCollectsThroughIntermediateOwners(t *testing.T) {
	h := newGraph(t)
	rs, d := eid(kube.KindReplicaSet, "ns", "rs"),
		eid(kube.KindDeployment, "ns", "d")
	h.relateOwned(podID("p1"), rs)
	h.relateOwned(podID("p2"), rs)
	h.relate(rs, inventory.OwnedBy, d)

	if got := OwnedPods(h.model, d); len(got) != 2 {
		t.Errorf("owned = %v", got)
	}
	p := podID("p1")
	if got := OwnedPods(h.model, p); len(got) != 1 || got[0] != p {
		t.Errorf("pod owns itself, got %v", got)
	}
}
