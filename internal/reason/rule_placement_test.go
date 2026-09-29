package reason

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

var (
	rsID = eid(kube.KindReplicaSet, "ns", "rs")
	n1   = eid(kube.KindNode, "", "n1")
	n2   = eid(kube.KindNode, "", "n2")
)

// placed puts a pod owned by rs on a node.
func (h *harness) placed(name string, on knowledge.EntityID) {
	h.ownedPod(podID(name), rsID)
	h.relate(podID(name), knowledge.RunsOn, on)
}

func nodeScenario(t *testing.T) *harness {
	h := newHarness(t, t0)
	h.placed("p1", n1)
	h.placed("p2", n2)
	return h
}

func TestNodeRuleHealthyReplicasElsewhereRaiseScore(t *testing.T) {
	h := nodeScenario(t)
	h.signal(n1, "MemoryPressure", t0.Add(-5*time.Minute), signal.Warning)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := NodeRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score < High {
		t.Fatalf("hyps = %+v", hyps)
	}
}

func TestNodeRuleFailingReplicasElsewhereContradict(t *testing.T) {
	h := nodeScenario(t)
	h.signal(n1, "MemoryPressure", t0.Add(-5*time.Minute), signal.Warning)
	h.signal(podID("p2"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := NodeRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score >= High {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score >= 0.5 {
		t.Errorf("contradiction should lower the score: %v",
			hyps[0].Score)
	}
}

func TestNodeRulePodFailingBeforeNodeProblemContradicts(t *testing.T) {
	h := nodeScenario(t)
	h.signal(n1, "MemoryPressure", t0.Add(5*time.Minute), signal.Warning)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := NodeRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 {
		t.Fatalf("hyps = %+v", hyps)
	}
	for _, p := range hyps[0].Points {
		if !p.Supports && p.Weight == 0.15 {
			return
		}
	}
	t.Errorf("missing ordering contradiction: %+v", hyps[0].Points)
}

func TestNodeRuleUsesEarliestNodeSignal(t *testing.T) {
	h := nodeScenario(t)
	h.signal(n1, "DiskPressure", t0.Add(-time.Minute), signal.Warning)
	h.signal(n1, "MemoryPressure", t0.Add(-9*time.Minute), signal.Warning)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := NodeRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Summary != "node n1: MemoryPressure" {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestNodeRuleNeedsPlacement(t *testing.T) {
	h := newHarness(t, t0)
	h.observe(podID("p1"), nil)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)
	if hyps := (NodeRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("unscheduled pod: %+v", hyps)
	}
}

// zones builds z1{n1,n2} and z2{n3}; a sibling runs on n3.
func zones(t *testing.T) *harness {
	h := newHarness(t, t0)
	n3 := eid(kube.KindNode, "", "n3")
	z1 := eid(kube.KindZone, "", "z1")
	z2 := eid(kube.KindZone, "", "z2")
	h.relate(n1, knowledge.PartOf, z1)
	h.relate(n2, knowledge.PartOf, z1)
	h.relate(n3, knowledge.PartOf, z2)
	h.placed("p1", n1)
	h.placed("p2", n2)
	h.placed("p3", n3)
	return h
}

func TestTopologyRuleBlamesZoneWhenReplicasOutsideAreHealthy(t *testing.T) {
	h := zones(t)
	h.signal(podID("p2"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := TopologyRule{}.Explain(h.query(), sym)
	z1 := eid(kube.KindZone, "", "z1")
	if len(hyps) != 1 || hyps[0].Root != z1 {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High ||
		hyps[0].Summary != "zone z1: 2 of 2 pods failing" {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestTopologyRuleFailureOutsideZoneLowersScore(t *testing.T) {
	h := zones(t)
	h.signal(podID("p2"), "Failed", t0, signal.Critical)
	h.signal(podID("p3"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := TopologyRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score >= High {
		t.Errorf("no outside-healthy bonus expected: %+v", hyps)
	}
}

func TestTopologyRuleNeedsSeveralFailingNodes(t *testing.T) {
	h := zones(t)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)
	if hyps := (TopologyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("single failing node blamed zone: %+v", hyps)
	}
}

func TestTopologyRuleMostlyHealthyZoneIsNotBlamed(t *testing.T) {
	h := zones(t)
	for _, name := range []string{"h1", "h2", "h3"} {
		h.placed(name, n1)
	}
	h.signal(podID("p2"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)
	if hyps := (TopologyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestTopologyRuleBoundsSampledPods(t *testing.T) {
	h := zones(t)
	for i := 0; i < maxTopologyPods+5; i++ {
		h.placed("bulk"+time.Duration(i).String(), n1)
	}
	h.signal(podID("p2"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)
	if hyps := (TopologyRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestTopologyRuleNeedsNodeAndPodSymptom(t *testing.T) {
	h := newHarness(t, t0)
	h.observe(podID("p1"), nil)
	sym := h.signal(podID("p1"), "Failed", t0, 0)
	if hyps := (TopologyRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
	svc := h.signal(eid(kube.KindService, "ns", "s"), "x", t0, 0)
	if hyps := (TopologyRule{}).Explain(h.query(), svc); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}
