package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

// announcedCount counts the announcements of incidents delivered so far.
func announcedCount(h *harness) int {
	count := 0
	for _, d := range h.decisions {
		if d.Action == incident.Announce && d.Incident.ID != "" {
			count++
		}
	}
	return count
}

// A workload breaks during its maintenance window and is still broken
// when the window ends. Nothing about the failure changes at that
// moment, so no decision asks the scope again; before, people first
// heard of it from its reminder, up to a week later. Now the scope is
// asked on every tick and the incident is announced when it comes back.
func TestFailureThatOutlivesItsMaintenanceWindowIsAnnounced(t *testing.T) {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	maintenance := true
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.InScope = func(incident.Incident) bool { return !maintenance }
	})
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 0, 0
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("payments-1", rs.Name, "n1", start))
	h.run(start.Add(30*time.Minute), 10*time.Second)
	require.Zero(t, announcedCount(h), "the window drops the announcement")
	require.Len(t, h.engine.deps.Incidents.Unheard(), 1)

	maintenance = false
	h.run(h.now.Add(time.Minute), 10*time.Second)

	require.Equal(t, 1, announcedCount(h), "announced when the window ends")
	assert.Empty(t, h.engine.deps.Incidents.Unheard())
	h.run(h.now.Add(30*time.Minute), 10*time.Second)
	assert.Equal(t, 1, announcedCount(h), "and only once")
}

// The coverage hand-back runs the same Apply as the solve step. A panic
// in it is contained as it is there: it must not end the process, which
// would restart into the same panic.
func TestCoverageHandBackContainsAPanic(t *testing.T) {
	h, id := crashingHarness(t)
	loseIncidents(h)
	// What the hand-back calls panics from here on.
	h.engine.announcer.incidents = nil
	panics := metrics.DefaultRegistry().PanicCount("incident")

	require.NotPanics(t, func() {
		h.run(h.now.Add(25*time.Minute), 10*time.Second)
	})

	assert.False(t, h.engine.coverage.Settled(id, h.now),
		"the workload was handed back")
	assert.Greater(t, metrics.DefaultRegistry().PanicCount("incident"),
		panics, "counted")
}
