package core

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/problem"
)

// A workload crashes and recovers every few minutes. After the first
// announcement it becomes one flapping problem: no stream of alerts and
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

	counts := map[problem.Action]int{}
	flapping := false
	for _, d := range h.decisions {
		counts[d.Action]++
		if d.Problem.State == problem.Flapping {
			flapping = true
		}
	}
	t.Logf("messages:\n%s", joinTitles(h))
	if counts[problem.Announce] != 1 {
		t.Fatalf("announced %d times, want once", counts[problem.Announce])
	}
	if !flapping {
		t.Fatal("expected the problem to be reported as flapping")
	}
	if counts[problem.Resolve] != 1 {
		t.Fatalf("resolved %d times, want once after it stabilised",
			counts[problem.Resolve])
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
