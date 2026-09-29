package reason

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestRolloutRuleTemplateChange(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	h.add(kube.DeploymentSchema(), deployment("deploy", "ns"))
	h.add(kube.ReplicaSetSchema(), replicaset("rs", "ns", "deploy"))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	rolloutTime := now.Add(-5 * time.Minute)
	deployID := knowledge.EntityID{Kind: kube.KindDeployment,
		Namespace: "ns", Name: "deploy"}
	h.model.Apply(knowledge.Fact{
		Kind: knowledge.Changed, Entity: deployID,
		Change: knowledge.Change{
			At: rolloutTime,
			Fields: []knowledge.FieldChange{
				{Path: "containers[app].image", Before: "app:1", After: "app:2"},
			},
		},
	})

	rule := RolloutRule{}
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) == 0 {
		t.Fatal("expected hypothesis for template change")
	}
	if hyps[0].Change == nil {
		t.Error("expected change in hypothesis")
	}
	if hyps[0].Score < DefaultFloor {
		t.Errorf("score below floor: %v", hyps[0].Score)
	}
}

func TestRolloutRuleIgnoresReplicaOnlyChange(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	h.add(kube.DeploymentSchema(), deployment("deploy", "ns"))
	h.add(kube.ReplicaSetSchema(), replicaset("rs", "ns", "deploy"))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	rolloutTime := now.Add(-5 * time.Minute)
	deployID := knowledge.EntityID{Kind: kube.KindDeployment,
		Namespace: "ns", Name: "deploy"}
	h.model.Apply(knowledge.Fact{
		Kind: knowledge.Changed, Entity: deployID,
		Change: knowledge.Change{
			At: rolloutTime,
			Fields: []knowledge.FieldChange{
				{Path: "spec.replicas"},
			},
		},
	})

	rule := RolloutRule{}
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) > 0 {
		t.Error("expected no hypothesis for replica-only change")
	}
}

func TestRolloutRuleWindowEnforcement(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, now)

	h.add(kube.DeploymentSchema(), deployment("deploy", "ns"))
	h.add(kube.ReplicaSetSchema(), replicaset("rs", "ns", "deploy"))
	h.add(kube.PodSchema{}, pod("p1", "ns", "rs", "n1", false, now))

	rolloutTime := now.Add(-20 * time.Minute)
	deployID := knowledge.EntityID{Kind: kube.KindDeployment,
		Namespace: "ns", Name: "deploy"}
	h.model.Apply(knowledge.Fact{
		Kind: knowledge.Changed, Entity: deployID,
		Change: knowledge.Change{
			At: rolloutTime,
			Fields: []knowledge.FieldChange{
				{Path: "containers[app].image", Before: "app:1", After: "app:2"},
			},
		},
	})

	rule := RolloutRule{}
	podID := knowledge.EntityID{Kind: kube.KindPod,
		Namespace: "ns", Name: "p1"}
	symptom := h.signal(podID, "Failed", now, signal.Critical)

	hyps := rule.Explain(h.query(), symptom)
	if len(hyps) > 0 {
		t.Error("expected no hypothesis outside rollout window")
	}
}
