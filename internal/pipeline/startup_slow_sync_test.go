package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestEngineStartupSlowSyncHoldsAnnouncements: a cold start whose last
// source syncs long after the settle deadline must not announce the
// existing failures one by one; they wait for the startup summary.
func TestEngineStartupSlowSyncHoldsAnnouncements(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start,
		func(d *Dependencies) { d.Store = &memStore{} })
	if err := h.engine.restore(); err != nil {
		t.Fatal(err)
	}
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	for _, name := range []string{"queue", "cache", "web"} {
		d, rs := deployment(name)
		d.Status.ReadyReplicas = *d.Spec.Replicas
		h.add(kube.DeploymentSchema(), d)
		h.add(kube.ReplicaSetSchema(), rs)
		h.add(kube.PodSchema{},
			crashingPod(name+"-a", rs.Name, "n1", start))
	}

	h.run(start.Add(5*time.Minute), 10*time.Second)
	for i, m := range h.messages {
		if !strings.HasPrefix(m.Key, "startup/") {
			t.Fatalf("announced before sources synced: %s",
				h.decisions[i].Incident.ID)
		}
	}

	h.engine.reconcileDowntime()
	completed := func() bool {
		return h.engine.announcer.collect.Startup.Summary.Complete
	}
	if !h.runUntil(completed, h.now.Add(5*time.Minute),
		10*time.Second) {
		t.Fatalf("summary never completed:\n%s", joinTitles(h))
	}
	if len(h.messages) != 1 ||
		!strings.HasPrefix(h.messages[0].Key, "startup/") {
		t.Fatalf("want one startup summary:\n%s", joinTitles(h))
	}
	if n := len(h.engine.announcer.collect.Startup.Summary.Incidents); n < 1 {
		t.Fatalf("summary lists %d incidents", n)
	}
}
