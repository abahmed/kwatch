package reason

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// consumers makes n pods reference cm; the first is the symptom pod.
func consumers(t *testing.T, n int) (*harness, knowledge.EntityID) {
	h := newHarness(t, t0)
	cm := eid(kube.KindConfigMap, "ns", "app-config")
	h.observe(cm, nil)
	for i := 0; i < n; i++ {
		p := podID("p" + string(rune('1'+i)))
		h.observe(p, nil)
		h.relate(p, knowledge.References, cm)
	}
	return h, cm
}

func TestConfigRuleBlamesDeletedConfig(t *testing.T) {
	h, cm := consumers(t, 1)
	h.change(cm, t0.Add(-5*time.Minute), false, field("data.K", "a", ""))
	h.model.Apply(knowledge.Fact{Kind: knowledge.Changed, Entity: cm,
		Change: knowledge.Change{Entity: cm, Deleted: true,
			At: t0.Add(-time.Minute)}})
	sym := h.signal(podID("p1"), "CreateContainerConfigError", t0, 0)

	hyps := ConfigRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != cm {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < DefaultFloor || hyps[0].Change == nil {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestConfigRuleErrorNamingChangedKeyScoresAboveFloor(t *testing.T) {
	h, cm := consumers(t, 1)
	h.change(cm, t0.Add(-5*time.Minute), false,
		field("data.LOG_LEVEL", "a", "b"))
	sym := h.evidenceSignal(podID("p1"), "Failed",
		"missing LOG_LEVEL value", t0)

	hyps := ConfigRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score < DefaultFloor {
		t.Fatalf("hyps = %+v", hyps)
	}
	if !strings.Contains(hyps[0].Summary, "configmap app-config") {
		t.Errorf("summary = %q", hyps[0].Summary)
	}
}

func TestConfigRuleWeakEvidenceStaysBelowFloor(t *testing.T) {
	h, cm := consumers(t, 1)
	h.change(cm, t0.Add(-5*time.Minute), false, field("data.K", "a", "b"))
	sym := h.signal(podID("p1"), "Failed", t0, 0)

	hyps := ConfigRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score >= DefaultFloor {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestConfigRuleSharedByHealthyPodsContradicts(t *testing.T) {
	h, cm := consumers(t, 3)
	h.change(cm, t0.Add(-5*time.Minute), false, field("data.K", "a", ""))
	h.model.Apply(knowledge.Fact{Kind: knowledge.Changed, Entity: cm,
		Change: knowledge.Change{Entity: cm, Deleted: true,
			At: t0.Add(-time.Minute)}})
	sym := h.signal(podID("p1"), "Failed", t0, 0)
	alone := ConfigRule{}.Explain(h.query(), sym)

	h.signal(podID("p2"), "Failed", t0, 0)
	h.signal(podID("p3"), "Failed", t0, 0)
	all := ConfigRule{}.Explain(h.query(), sym)
	if len(alone) != 1 || len(all) != 1 || all[0].Score <= alone[0].Score {
		t.Errorf("alone=%+v all=%+v", alone, all)
	}
}

func TestConfigRuleIgnoresCreationAndOldChanges(t *testing.T) {
	h, cm := consumers(t, 1)
	sym := h.signal(podID("p1"), "Failed", t0, 0)
	h.change(cm, t0.Add(-time.Minute), true)
	if hyps := (ConfigRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("creation blamed: %+v", hyps)
	}
	h.change(cm, t0.Add(-2*time.Hour), false, field("data.K", "a", "b"))
	if hyps := (ConfigRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("stale change blamed: %+v", hyps)
	}
}

func TestConfigRuleDefersToRolloutRelease(t *testing.T) {
	h, cm := consumers(t, 1)
	d := eid(kube.KindDeployment, "ns", "d")
	h.observe(d, nil)
	h.relate(podID("p1"), knowledge.OwnedBy, d)
	at := t0.Add(-5 * time.Minute)
	h.change(cm, at, false, field("data.K", "a", ""))
	imageChange(h, d, at.Add(30*time.Second))
	sym := h.signal(podID("p1"), "Failed", t0, 0)

	if hyps := (ConfigRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("release change blamed on config: %+v", hyps)
	}
}

func TestConfigRuleNonPodSymptom(t *testing.T) {
	h := newHarness(t, t0)
	sym := h.signal(eid(kube.KindNode, "", "n"), "x", t0, 0)
	if hyps := (ConfigRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestReferenceRuleBlamesMissingSecret(t *testing.T) {
	h := newHarness(t, t0)
	sec := eid(kube.KindSecret, "ns", "db-creds")
	h.observe(podID("p1"), nil)
	h.relate(podID("p1"), knowledge.References, sec)
	sym := h.evidenceSignal(podID("p1"), "Failed",
		`secret "db-creds" not found`, t0)

	hyps := ReferenceRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != sec {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High ||
		!strings.Contains(hyps[0].Summary, "does not exist") {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestReferenceRuleBlamesUnhealthyVolumeClaim(t *testing.T) {
	h := newHarness(t, t0)
	pvc := eid(kube.KindPVC, "ns", "data")
	h.observe(podID("p1"), nil)
	h.observe(pvc, nil)
	h.relate(podID("p1"), knowledge.Mounts, pvc)
	h.signal(pvc, "Pending", t0, signal.Warning)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	hyps := ReferenceRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != pvc ||
		hyps[0].Score < DefaultFloor {
		t.Fatalf("hyps = %+v", hyps)
	}
	if len(hyps[0].RootSignals) != 1 {
		t.Error("root signals should carry the claim's problem")
	}
}

func TestReferenceRuleHealthyExistingDependencyIsSilent(t *testing.T) {
	h := newHarness(t, t0)
	cm := eid(kube.KindConfigMap, "ns", "ok")
	h.observe(podID("p1"), nil)
	h.observe(cm, nil)
	h.relate(podID("p1"), knowledge.References, cm)
	sym := h.signal(podID("p1"), "Failed", t0, signal.Critical)

	if hyps := (ReferenceRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("hyps = %+v", hyps)
	}
	if hyps := (ReferenceRule{}).Explain(h.query(),
		h.signal(eid(kube.KindNode, "", "n"), "x", t0, 0)); hyps != nil {
		t.Errorf("non-pod hyps = %+v", hyps)
	}
}

func TestMentionsNameNeedsLongEnoughName(t *testing.T) {
	sym := signal.Signal{Summary: "no ab here"}
	if mentionsName(sym, "ab") {
		t.Error("two letter names are too ambiguous")
	}
	if !mentionsName(signal.Signal{Summary: "Missing DBX"}, "dbx") {
		t.Error("three letter names are matched case-insensitively")
	}
}

func TestPodsRuleExplainsControllerByFailingPod(t *testing.T) {
	h := newHarness(t, t0)
	rs, d := eid(kube.KindReplicaSet, "ns", "rs"),
		eid(kube.KindDeployment, "ns", "d")
	h.ownedPod(podID("p1"), rs)
	h.ownedPod(podID("p2"), rs)
	h.relate(rs, knowledge.OwnedBy, d)
	c := eid(kube.KindContainer, "ns", "app")
	h.relate(c, knowledge.PartOf, podID("p2"))
	h.signal(c, "OOMKilled", t0, signal.Critical)
	sym := h.signal(d, "Unavailable", t0, signal.Critical)

	hyps := PodsRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != podID("p2") {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Summary != "OOMKilled" || hyps[0].Score < Likely {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestPodsRulePrefersPodOwnSignalsAndSkipsHealthy(t *testing.T) {
	h := newHarness(t, t0)
	rs := eid(kube.KindReplicaSet, "ns", "rs")
	h.ownedPod(podID("p1"), rs)
	sym := h.signal(rs, "Unavailable", t0, signal.Critical)
	if hyps := (PodsRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("healthy pods explained: %+v", hyps)
	}
	h.signal(podID("p1"), "Failed", t0, signal.Critical)
	hyps := PodsRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != podID("p1") {
		t.Errorf("hyps = %+v", hyps)
	}
	svc := h.signal(eid(kube.KindService, "ns", "s"), "x", t0, 0)
	if hyps := (PodsRule{}).Explain(h.query(), svc); hyps != nil {
		t.Errorf("non-controller explained: %+v", hyps)
	}
}

func TestBackendRuleBlamesWorkloadBehindService(t *testing.T) {
	h := newHarness(t, t0)
	svc := eid(kube.KindService, "ns", "web")
	slice := eid(kube.KindEndpointSlice, "ns", "web-abc")
	rs := eid(kube.KindReplicaSet, "ns", "rs")
	h.ownedPod(podID("p1"), rs)
	h.ownedPod(podID("p2"), rs)
	h.relate(slice, knowledge.Backs, svc)
	h.relate(slice, knowledge.RoutesTo, podID("p1"), podID("p2"))
	h.signal(podID("p1"), "Failed", t0, signal.Critical)
	sym := h.signal(svc, "NoEndpoints", t0, signal.Critical)

	hyps := BackendRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != rs {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High ||
		hyps[0].Summary != "replicaset rs has no ready pods" {
		t.Errorf("hyp = %+v", hyps[0])
	}
}

func TestBackendRuleSilentWhenBackendsHealthy(t *testing.T) {
	h := newHarness(t, t0)
	svc := eid(kube.KindService, "ns", "web")
	slice := eid(kube.KindEndpointSlice, "ns", "web-abc")
	h.ownedPod(podID("p1"), eid(kube.KindReplicaSet, "ns", "rs"))
	h.relate(slice, knowledge.Backs, svc)
	h.relate(slice, knowledge.RoutesTo, podID("p1"))
	sym := h.signal(svc, "NoEndpoints", t0, signal.Critical)
	if hyps := (BackendRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("hyps = %+v", hyps)
	}
	other := h.signal(podID("p1"), "x", t0, 0)
	if hyps := (BackendRule{}).Explain(h.query(), other); hyps != nil {
		t.Errorf("non-service symptom: %+v", hyps)
	}
}
