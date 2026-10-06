package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/notification"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node under memory pressure makes pods of three workloads unready while
// their replicas on another node stay healthy. The node is the one root:
// no incident is announced per workload.
func TestEngineNodePressureIsOneIncident(t *testing.T) {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", start), node("n2", time.Time{}))
	for _, name := range []string{"orders", "payments", "cart"} {
		d, rs := deployment(name)
		h.add(kube.DeploymentSchema(), d)
		h.add(kube.ReplicaSetSchema(), rs)
		h.add(kube.PodSchema{},
			pod(name+"-a", rs.Name, "n1", false, start.Add(10*time.Second)),
			pod(name+"-b", rs.Name, "n2", true, start),
		)
	}

	h.run(start.Add(10*time.Minute), 5*time.Second)

	if len(h.decisions) == 0 {
		t.Fatal("expected the node incident to be announced")
	}
	for _, d := range h.decisions {
		if d.Incident.Root.Kind != kube.KindNode ||
			d.Incident.Root.Name != "n1" {
			t.Fatalf("decision rooted at %s, want node n1",
				d.Incident.Root)
		}
	}
	if h.decisions[0].Action != incident.Announce {
		t.Fatalf("first decision = %v, want announce", h.decisions[0].Action)
	}
	if len(h.decisions) > 3 {
		t.Fatalf("got %d messages for one node incident, want at most 3",
			len(h.decisions))
	}
	t.Logf("messages:\n%s", joinTitles(h))
	t.Logf("last message:\n%s", notification.Text(h.messages[len(h.messages)-1]))
}

func joinTitles(h *harness) string {
	out := ""
	for i, m := range h.messages {
		out += string(h.decisions[i].Reason) + ": " + m.Title + "\n"
	}
	return out
}
