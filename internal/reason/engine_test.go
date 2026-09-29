package reason

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestExplainDropsHypothesesRootedAtSymptom(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	engine := NewEngine(0, &fakeRule{})
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	exp := engine.Explain(h.query(), symptom)
	if exp.Cause != nil {
		t.Error("expected nil cause for symptom rooted at same entity")
	}
}

func TestExplainRespectsCandidateFloor(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))
	h.add(kube.NodeSchema{}, node("n1", now))

	nodeID := knowledge.EntityID{Kind: kube.KindNode, Name: "n1"}
	engine := NewEngine(0.7, &fakeRule{
		hypotheses: []Hypothesis{
			{Root: nodeID, Score: 0.4, Summary: "low score"},
		},
	})
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	exp := engine.Explain(h.query(), symptom)
	if exp.Cause != nil {
		t.Error("expected nil cause below floor")
	}
	if len(exp.Rejected) == 0 {
		t.Error("expected rejected hypothesis below floor")
	}
}

func TestExplainSortsByScoreDescending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))
	h.add(kube.NodeSchema{}, node("n1", now))
	h.add(kube.NodeSchema{}, node("n2", now.Add(-1*time.Hour)))

	nodeID1 := knowledge.EntityID{Kind: kube.KindNode, Name: "n1"}
	nodeID2 := knowledge.EntityID{Kind: kube.KindNode, Name: "n2"}

	engine := NewEngine(0, &fakeRule{
		hypotheses: []Hypothesis{
			{Root: nodeID1, Score: 0.5},
			{Root: nodeID2, Score: 0.8},
		},
	})
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	exp := engine.Explain(h.query(), symptom)
	if exp.Cause == nil || exp.Cause.Root != nodeID2 {
		t.Error("expected highest score as cause")
	}
	if len(exp.Alternatives) > 0 &&
		exp.Alternatives[0].Score > exp.Cause.Score {
		t.Error("alternatives not sorted descending")
	}
}

func TestExplainNilCauseWhenNoRulesApply(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	engine := NewEngine(0)
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	exp := engine.Explain(h.query(), symptom)
	if exp.Cause != nil {
		t.Error("expected nil cause with no rules")
	}
	if exp.Symptom.Entity != symptom.Entity {
		t.Error("expected symptom entity preserved")
	}
}

type fakeRule struct {
	score      float64
	hypotheses []Hypothesis
}

func (r *fakeRule) Name() string { return "fake" }

func (r *fakeRule) Explain(_ Query, symptom signal.Signal) []Hypothesis {
	if len(r.hypotheses) > 0 {
		return r.hypotheses
	}
	return []Hypothesis{{
		Root: symptom.Entity, Score: r.score,
		Summary: "fake hypothesis",
	}}
}
