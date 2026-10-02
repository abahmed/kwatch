package pipeline

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A workload crashes and recovers every few minutes. After the first
// announcement it becomes one flapping incident: no stream of alerts and
// "recovered" messages.
func TestEngineFlappingWorkloadIsQuiet(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("queue")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	healthy := pod("queue-a", rs.Name, "n1", true, start)
	h.add(kube.PodSchema{}, healthy)
	translator := kube.NewTranslator(kube.PodSchema{})

	current := healthy
	for cycle := 0; cycle < 6; cycle++ {
		crashing := crashingPod("queue-a", rs.Name, "n1", h.now)
		crashing.Status.ContainerStatuses[0].Image = "app:1"
		h.engine.Submit(ctxBackground(),
			translator.Updated(current, crashing, h.now)...)
		h.run(h.now.Add(2*time.Minute), 10*time.Second)
		recovered := running(healthy)
		h.engine.Submit(ctxBackground(),
			translator.Updated(crashing, recovered, h.now)...)
		h.run(h.now.Add(2*time.Minute), 10*time.Second)
		current = recovered
	}
	h.run(h.now.Add(40*time.Minute), 30*time.Second)

	counts := map[incident.Action]int{}
	flapping := false
	for _, d := range h.decisions {
		counts[d.Action]++
		if d.Incident.State == incident.Flapping {
			flapping = true
		}
	}
	t.Logf("messages:\n%s", joinTitles(h))
	if counts[incident.Announce] != 1 {
		t.Fatalf("announced %d times, want once", counts[incident.Announce])
	}
	if !flapping {
		t.Fatal("expected the incident to be reported as flapping")
	}
	if counts[incident.Resolve] != 1 {
		t.Fatalf("resolved %d times, want once after it stabilised",
			counts[incident.Resolve])
	}
	if len(h.decisions) > 4 {
		t.Fatalf("got %d messages for one flapping workload",
			len(h.decisions))
	}
}

func running(p *corev1.Pod) *corev1.Pod {
	out := p.DeepCopy()
	out.ResourceVersion = "next"
	return out
}

func TestEngineRecoveredIncidentResolvesAtHoldDeadline(t *testing.T) {
	start := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
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
	h.run(h.now.Add(2*time.Minute), 10*time.Second)
	h.engine.Submit(ctxBackground(),
		translator.Updated(crashing, running(healthy), h.now)...)

	var recoveringAt time.Time
	for i := 0; i < 50; i++ {
		before := len(h.decisions)
		next, _ := h.engine.step(ctxBackground(), h.now, h.checks)
		if recoveringAt.IsZero() && h.engine.deps.Incidents.Export()[0].
			State == incident.Recovering {
			recoveringAt = h.now
		}
		if len(h.decisions) > before {
			last := h.decisions[len(h.decisions)-1]
			if last.Action != incident.Resolve {
				t.Fatalf("unexpected decision %v", last.Action)
			}
			if got := h.now.Sub(recoveringAt); got != incident.DefaultHold {
				t.Fatalf("resolved after %v, want hold %v",
					got, incident.DefaultHold)
			}
			return
		}
		if next.IsZero() || !next.After(h.now) {
			t.Fatalf("step at %v reported no next wake", h.now)
		}
		h.now = next
	}
	t.Fatal("incident never resolved")
}
