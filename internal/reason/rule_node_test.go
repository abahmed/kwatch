package reason

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestNodeRuleBlamesUnhealthyNode(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	since := now.Add(-5 * time.Minute)
	h.add(kube.NodeSchema{}, node("n1", since))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	nodeID := knowledge.EntityID{Kind: kube.KindNode, Name: "n1"}
	h.signal(nodeID, "MemoryPressure", since, signal.Warning)

	rule := NodeRule{}
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) == 0 {
		t.Fatal("expected hypothesis blaming node")
	}
	if hyps[0].Root != nodeID {
		t.Errorf("expected root %v, got %v", nodeID, hyps[0].Root)
	}
	if hyps[0].Score < DefaultFloor {
		t.Errorf("score below floor: %v", hyps[0].Score)
	}
}

func TestNodeRuleIgnoresHealthyNode(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	rule := NodeRule{}
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) > 0 {
		t.Error("expected no hypothesis for healthy node")
	}
}

func TestNodeRuleNonPodSymptom(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	h.add(kube.NodeSchema{}, node("n1", now))

	rule := NodeRule{}
	svcID := knowledge.EntityID{Kind: kube.KindService,
		Namespace: "ns", Name: "svc"}
	symptom := h.signal(svcID, "Unreachable", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) > 0 {
		t.Error("expected no hypothesis for non-pod symptom")
	}
}

func TestNodeRuleContainerSymptom(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	since := now.Add(-5 * time.Minute)
	h.add(kube.NodeSchema{}, node("n1", since))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	containerID := knowledge.EntityID{Kind: kube.KindContainer,
		Namespace: "ns", Name: "app"}
	h.model.Apply(knowledge.Fact{
		Kind:     knowledge.Related,
		Entity:   containerID,
		Relation: knowledge.PartOf,
		Targets:  []knowledge.EntityID{podID},
	})

	nodeID := knowledge.EntityID{Kind: kube.KindNode, Name: "n1"}
	h.signal(nodeID, "MemoryPressure", since, signal.Warning)

	rule := NodeRule{}
	symptom := h.signal(containerID, "OOMKilled", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) == 0 {
		t.Fatal("expected hypothesis for container in pod")
	}
	if hyps[0].Root != nodeID {
		t.Errorf("expected root %v, got %v", nodeID, hyps[0].Root)
	}
}
