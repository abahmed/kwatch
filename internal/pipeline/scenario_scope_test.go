package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func scopedHarness(t *testing.T, start time.Time) *harness {
	t.Helper()
	return newHarnessWith(t, start, func(d *Dependencies) {
		d.InScope = IncidentScope(d.Model, namespaceScope("shop"))
	})
}

func wantAnnounceThenResolve(t *testing.T, h *harness) {
	t.Helper()
	counts := map[incident.Action]int{}
	for _, d := range h.decisions {
		counts[d.Action]++
	}
	if counts[incident.Announce] != 1 || counts[incident.Resolve] != 1 {
		t.Fatalf("want one announce and one resolve, got %v:\n%s",
			counts, joinTitles(h))
	}
	if last := h.decisions[len(h.decisions)-1]; last.Action !=
		incident.Resolve {
		t.Fatalf("last decision = %v, want resolve", last.Action)
	}
}

// A crash with no upstream cause is announced in scope. Its resolve has
// no member left to judge scope by and must still be delivered.
func TestEngineScopedNoCauseCrashDeliversResolve(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h := scopedHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("queue")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	healthy := pod("queue-a", rs.Name, "n1", true, start)
	h.add(kube.PodSchema{}, healthy)
	translator := kube.NewTranslator(kube.PodSchema{})
	crashing := crashingPod("queue-a", rs.Name, "n1", h.now)
	h.engine.Submit(ctxBackground(),
		translator.Updated(healthy, crashing, h.now)...)
	h.run(h.now.Add(3*time.Minute), 10*time.Second)
	h.engine.Submit(ctxBackground(),
		translator.Updated(crashing, running(healthy), h.now)...)

	h.run(h.now.Add(10*time.Minute), 10*time.Second)

	wantAnnounceThenResolve(t, h)
}

// A rollout-rooted incident keeps its scope through the resolve after the
// broken revision is fixed.
func TestEngineScopedRolloutIncidentDeliversResolve(t *testing.T) {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	h := scopedHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	old, oldRS := deployment("payments")
	old.Spec.Template = podTemplate("app:2.2")
	h.add(kube.DeploymentSchema(), old)
	h.add(kube.ReplicaSetSchema(), oldRS)
	h.add(kube.PodSchema{}, pod("payments-old", oldRS.Name, "n1", true,
		start))
	h.run(start.Add(time.Minute), 10*time.Second)
	updated := old.DeepCopy()
	updated.Spec.Template = podTemplate("app:2.3")
	h.engine.Submit(ctxBackground(), kube.NewTranslator(
		kube.DeploymentSchema()).Updated(old, updated, h.now)...)
	newRS := replicaSet("payments-9c", "payments")
	h.add(kube.ReplicaSetSchema(), newRS)
	crashing := crashingPod("payments-new", newRS.Name, "n1",
		h.now.Add(40*time.Second))
	h.add(kube.PodSchema{}, crashing)
	h.run(start.Add(6*time.Minute), 10*time.Second)
	if len(h.decisions) == 0 || h.decisions[0].Incident.Cause == nil ||
		h.decisions[0].Incident.Cause.Change == nil {
		t.Fatalf("rollout incident not announced with its change:\n%s",
			joinTitles(h))
	}

	fixed := pod("payments-new", newRS.Name, "n1", true, h.now)
	fixed.ResourceVersion = "next"
	h.engine.Submit(ctxBackground(), kube.NewTranslator(
		kube.PodSchema{}).Updated(crashing, fixed, h.now)...)
	settled := updated.DeepCopy()
	settled.ResourceVersion = "next"
	settled.Status.ReadyReplicas = *settled.Spec.Replicas
	settled.Status.AvailableReplicas = *settled.Spec.Replicas
	h.engine.Submit(ctxBackground(), kube.NewTranslator(
		kube.DeploymentSchema()).Updated(updated, settled, h.now)...)
	h.run(h.now.Add(40*time.Minute), 10*time.Second)

	wantAnnounceThenResolve(t, h)
}
