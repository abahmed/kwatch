package reason

import (
	"testing"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestPodOfResolvesPodAndContainer(t *testing.T) {
	h := newHarness(t, t0)
	p := podID("p1")
	c := eid(kube.KindContainer, "ns", "app")
	h.observe(p, nil)
	h.relate(c, knowledge.PartOf, p)

	if got, ok := PodOf(h.model, p); !ok || got != p {
		t.Errorf("pod: got %v %v", got, ok)
	}
	if got, ok := PodOf(h.model, c); !ok || got != p {
		t.Errorf("container: got %v %v", got, ok)
	}
}

func TestPodOfRejectsOrphanContainerAndOtherKinds(t *testing.T) {
	h := newHarness(t, t0)
	orphan := eid(kube.KindContainer, "ns", "lonely")
	if _, ok := PodOf(h.model, orphan); ok {
		t.Error("orphan container has no pod")
	}
	if _, ok := PodOf(h.model, eid(kube.KindNode, "", "n1")); ok {
		t.Error("node is not a pod")
	}
}

func TestOwnerChainWalksToTopController(t *testing.T) {
	h := newHarness(t, t0)
	p, rs, d := podID("p1"), eid(kube.KindReplicaSet, "ns", "rs"),
		eid(kube.KindDeployment, "ns", "d")
	h.ownedPod(p, rs)
	h.relate(rs, knowledge.OwnedBy, d)

	chain := ownerChain(h.model, p)
	if len(chain) != 2 || chain[0] != rs || chain[1] != d {
		t.Fatalf("chain = %v", chain)
	}
	if TopOwner(h.model, p) != d {
		t.Error("top owner should be the deployment")
	}
}

func TestOwnerChainExcludesStaticPodNodeOwner(t *testing.T) {
	h := newHarness(t, t0)
	p, n := podID("static"), eid(kube.KindNode, "", "n1")
	h.ownedPod(p, n)

	if chain := ownerChain(h.model, p); len(chain) != 0 {
		t.Errorf("node must not be an owner, got %v", chain)
	}
	if TopOwner(h.model, p) != p {
		t.Error("static pod is its own top owner")
	}
}

func TestOwnerChainStopsOnCycle(t *testing.T) {
	h := newHarness(t, t0)
	a, b := eid(kube.KindReplicaSet, "ns", "a"),
		eid(kube.KindReplicaSet, "ns", "b")
	h.relate(a, knowledge.OwnedBy, b)
	h.relate(b, knowledge.OwnedBy, a)

	if chain := ownerChain(h.model, a); len(chain) != 1 || chain[0] != b {
		t.Errorf("cycle chain = %v", chain)
	}
}

func TestOwnedPodsCollectsThroughIntermediateOwners(t *testing.T) {
	h := newHarness(t, t0)
	rs, d := eid(kube.KindReplicaSet, "ns", "rs"),
		eid(kube.KindDeployment, "ns", "d")
	h.ownedPod(podID("p1"), rs)
	h.ownedPod(podID("p2"), rs)
	h.relate(rs, knowledge.OwnedBy, d)

	if got := OwnedPods(h.model, d); len(got) != 2 {
		t.Errorf("owned = %v", got)
	}
	p := podID("p1")
	if got := OwnedPods(h.model, p); len(got) != 1 || got[0] != p {
		t.Errorf("pod owns itself, got %v", got)
	}
}

func TestSiblingPodsExcludeSelfAndHandleOrphans(t *testing.T) {
	h := newHarness(t, t0)
	rs := eid(kube.KindReplicaSet, "ns", "rs")
	h.ownedPod(podID("p1"), rs)
	h.ownedPod(podID("p2"), rs)
	h.observe(podID("solo"), nil)

	sib := siblingPods(h.model, podID("p1"))
	if len(sib) != 1 || sib[0] != podID("p2") {
		t.Errorf("siblings = %v", sib)
	}
	if got := siblingPods(h.model, podID("solo")); got != nil {
		t.Errorf("orphan siblings = %v", got)
	}
}

func TestFailingShareCountsContainerSignals(t *testing.T) {
	h := newHarness(t, t0)
	rs := eid(kube.KindReplicaSet, "ns", "rs")
	h.ownedPod(podID("p1"), rs)
	h.ownedPod(podID("p2"), rs)
	c := eid(kube.KindContainer, "ns", "app")
	h.relate(c, knowledge.PartOf, podID("p1"))
	h.signal(c, "OOMKilled", t0, 0)

	failing, total := failingShare(h.query(),
		[]knowledge.EntityID{podID("p1"), podID("p2")})
	if failing != 1 || total != 2 {
		t.Errorf("share = %d/%d", failing, total)
	}
}

func TestPodsOnNodeListsScheduledPods(t *testing.T) {
	h := newHarness(t, t0)
	n := eid(kube.KindNode, "", "n1")
	h.observe(podID("p1"), nil)
	h.relate(podID("p1"), knowledge.RunsOn, n)

	if got := podsOnNode(h.model, n); len(got) != 1 {
		t.Errorf("pods = %v", got)
	}
}
