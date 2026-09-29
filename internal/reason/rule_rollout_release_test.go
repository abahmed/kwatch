package reason

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// revisions builds deploy -> rsNew/rsOld with one pod each; the new pod
// is the symptom.
func revisions(t *testing.T) (*harness, knowledge.EntityID) {
	h := newHarness(t, t0)
	d := eid(kube.KindDeployment, "ns", "d")
	rsNew := eid(kube.KindReplicaSet, "ns", "rs-new")
	rsOld := eid(kube.KindReplicaSet, "ns", "rs-old")
	h.observe(d, nil)
	h.ownedPod(podID("new"), rsNew)
	h.ownedPod(podID("old"), rsOld)
	h.relate(rsNew, knowledge.OwnedBy, d)
	h.relate(rsOld, knowledge.OwnedBy, d)
	return h, d
}

func imageChange(h *harness, d knowledge.EntityID, at time.Time) {
	h.change(d, at, false, field("containers[app].image", "app:1", "app:2"))
}

func TestRolloutRuleNewRevisionOnlyScoresHigher(t *testing.T) {
	h, d := revisions(t)
	imageChange(h, d, t0.Add(-30*time.Second))
	sym := h.signal(podID("new"), "Failed", t0, signal.Critical)

	hyps := RolloutRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Root != d {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score < High {
		t.Errorf("new-revision-only score = %v", hyps[0].Score)
	}
	if !strings.Contains(hyps[0].Summary, "changed image app:1") {
		t.Errorf("summary = %q", hyps[0].Summary)
	}
}

func TestRolloutRuleOldRevisionFailingContradicts(t *testing.T) {
	h, d := revisions(t)
	imageChange(h, d, t0.Add(-3*time.Minute))
	h.signal(podID("old"), "Failed", t0, signal.Critical)
	sym := h.signal(podID("new"), "Failed", t0, signal.Critical)

	hyps := RolloutRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 {
		t.Fatalf("hyps = %+v", hyps)
	}
	if hyps[0].Score >= DefaultFloor {
		t.Errorf("contradicted score = %v", hyps[0].Score)
	}
}

func TestRolloutRuleMentionsChangedImageInError(t *testing.T) {
	h, d := revisions(t)
	imageChange(h, d, t0.Add(-8*time.Minute))
	sym := h.evidenceSignal(podID("new"), "Failed", "cannot pull app:2", t0)

	hyps := RolloutRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 || hyps[0].Score < DefaultFloor {
		t.Fatalf("hyps = %+v", hyps)
	}
}

func TestRolloutRuleIgnoresChangeAfterSymptom(t *testing.T) {
	h, d := revisions(t)
	imageChange(h, d, t0.Add(time.Minute))
	sym := h.signal(podID("new"), "Failed", t0, signal.Critical)

	if hyps := (RolloutRule{}).Explain(h.query(), sym); len(hyps) != 0 {
		t.Errorf("future change blamed: %+v", hyps)
	}
}

func TestRolloutRuleNamesCompanionConfigChanges(t *testing.T) {
	h, d := revisions(t)
	cm := eid(kube.KindConfigMap, "ns", "app-config")
	h.observe(cm, nil)
	h.relate(d, knowledge.References, cm)
	imageChange(h, d, t0.Add(-4*time.Minute))
	h.change(cm, t0.Add(-4*time.Minute-30*time.Second), false,
		field("data.KEY", "a", "b"))
	sym := h.signal(podID("new"), "Failed", t0, signal.Critical)

	hyps := RolloutRule{}.Explain(h.query(), sym)
	if len(hyps) != 1 {
		t.Fatalf("hyps = %+v", hyps)
	}
	want := "configmap app-config changed with it"
	if !strings.Contains(hyps[0].Summary, want) {
		t.Errorf("summary = %q", hyps[0].Summary)
	}
}

func TestRolloutRuleNoRolloutForNonPodSymptom(t *testing.T) {
	h, _ := revisions(t)
	svc := eid(kube.KindService, "ns", "svc")
	sym := h.signal(svc, "NoEndpoints", t0, signal.Critical)
	if hyps := (RolloutRule{}).Explain(h.query(), sym); hyps != nil {
		t.Errorf("hyps = %+v", hyps)
	}
}

func TestCompanionChangesSkipsCreatedAndDistantChanges(t *testing.T) {
	h := newHarness(t, t0)
	d := eid(kube.KindDeployment, "ns", "d")
	fresh := eid(kube.KindSecret, "ns", "fresh")
	far := eid(kube.KindConfigMap, "ns", "far")
	near := eid(kube.KindSecret, "ns", "near")
	other := eid(kube.KindService, "ns", "svc")
	h.relate(d, knowledge.References, fresh, far, near, other)
	at := t0.Add(-10 * time.Minute)
	imageChange(h, d, at)
	h.change(fresh, at, true)
	h.change(far, at.Add(-time.Minute), false, field("data.k", "", "x"))
	h.change(far, at.Add(ChangeSetWindow+time.Minute), false,
		field("data.k", "x", "y"))
	h.change(near, at.Add(time.Minute), false, field("data.k", "", "x"))

	changes := h.model.Changes(d, at.Add(-time.Minute))
	got := companionChanges(h.query(), d, changes[0])
	if len(got) != 2 || got[0] != far || got[1] != near {
		// far has an in-window change on its early side.
		t.Errorf("companions = %v", got)
	}
}

func TestDescribeCompanionsJoinsNames(t *testing.T) {
	if describeCompanions(nil) != "" {
		t.Error("no companions renders nothing")
	}
	got := describeCompanions([]knowledge.EntityID{
		eid(kube.KindConfigMap, "ns", "a"),
		eid(kube.KindSecret, "ns", "b"),
	})
	want := "; configmap a and secret b changed with it"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestRolloutNearDetectsReleaseChanges(t *testing.T) {
	h := newHarness(t, t0)
	d := eid(kube.KindDeployment, "ns", "d")
	imageChange(h, d, t0)
	if !rolloutNear(h.query(), d, t0.Add(time.Minute)) {
		t.Error("rollout within window should be near")
	}
	if rolloutNear(h.query(), d, t0.Add(time.Hour)) {
		t.Error("rollout an hour away is not near")
	}
}
