package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// heldHarness has a crashing workload whose scope refuses it while
// held: only for now (pending) or, with pending false, for good.
func heldHarness(
	t *testing.T, held, pending *bool,
) *harness {
	t.Helper()
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.InScope = func(incident.Incident) bool { return !*held }
		d.ScopePending = func(incident.Incident) bool {
			return *held && *pending
		}
	})
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("shop")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 0, 0
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("shop-1", rs.Name, "n1", start))
	return h
}

// While the scope waits for a Namespace's labels its refusal is not a
// verdict: nothing is logged as out of scope, and the incident is asked
// about again and announced once the scope lets it in.
func TestAnnouncementWaitingForTheScopeIsNotWithheld(t *testing.T) {
	out := captureLog(t)
	held, pending := true, true
	h := heldHarness(t, &held, &pending)
	h.run(h.now.Add(5*time.Minute), 10*time.Second)
	require.Zero(t, announcedCount(h))
	assert.NotContains(t, out(), "out of scope")
	require.Len(t, h.engine.deps.Incidents.Unheard(), 1, "asked again")

	held = false
	h.run(h.now.Add(time.Minute), 10*time.Second)

	assert.Equal(t, 1, announcedCount(h))
}

// When the wait ends in a refusal, that verdict is logged then, once.
func TestWaitingAnnouncementLogsTheVerdictOnce(t *testing.T) {
	out := captureLog(t)
	held, pending := true, true
	h := heldHarness(t, &held, &pending)
	h.run(h.now.Add(5*time.Minute), 10*time.Second)

	pending = false
	h.run(h.now.Add(5*time.Minute), 10*time.Second)

	assert.Zero(t, announcedCount(h))
	assert.Equal(t, 1, strings.Count(out(),
		"incident not announced: out of scope"))
}

// An incident that already has a decision in this tick's list is left
// to inScope: asking it back as well would announce it twice.
func TestBackInScopeSkipsAnIncidentDecidedThisTick(t *testing.T) {
	held, pending := true, false
	h := heldHarness(t, &held, &pending)
	h.run(h.now.Add(5*time.Minute), 10*time.Second)
	unheard := h.engine.deps.Incidents.Unheard()
	require.Len(t, unheard, 1)
	held = false
	a := h.engine.announcer

	decided := []incident.Decision{{Action: incident.Update,
		Incident: unheard[0]}}
	assert.Empty(t, a.backInScope(h.now, decided))
	assert.Len(t, a.backInScope(h.now, nil), 1)
}
