package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// crashingHarness has a Deployment with no ready replica and a pod that
// crash-loops, runs until its incident is announced and returns the
// Deployment's id.
func crashingHarness(t *testing.T) (*harness, inventory.EntityID) {
	t.Helper()
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 0, 0
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("payments-1", rs.Name, "n1", start))
	h.run(start.Add(10*time.Minute), 10*time.Second)
	id := inventory.CoreID(kube.KindDeployment, "shop", "payments")
	require.True(t, h.engine.deps.Incidents.CoveredWorkloads(h.now)[id],
		"the crash loop is announced")
	return h, id
}

// loseIncidents detaches every member of every incident while the
// findings stay active in the tracker, as a bug that lost an incident
// would.
func loseIncidents(h *harness) {
	var lost []detection.Transition
	for _, found := range h.engine.findings {
		for _, f := range found {
			lost = append(lost, detection.Transition{
				Kind: detection.Cleared, Finding: f})
		}
	}
	// The snapshot has no findings, so nothing places them again.
	snapshot := explain.NewSnapshot(h.engine.deps.Model,
		map[inventory.EntityID][]detection.Finding{}, nil, h.now)
	h.engine.deps.Incidents.Apply(snapshot, nil, lost)
}

// A failing workload whose incident was lost gets one again, within a
// few checks, and the incident is not resolved on the way.
func TestCoverageCheckRecoversALostIncident(t *testing.T) {
	h, id := crashingHarness(t)
	loseIncidents(h)
	require.False(t, h.engine.deps.Incidents.CoveredWorkloads(h.now)[id])

	h.run(h.now.Add(25*time.Minute), 10*time.Second)

	assert.True(t, h.engine.deps.Incidents.CoveredWorkloads(h.now)[id],
		"the backstop covered the workload again")
	assert.False(t, h.engine.coverage.Settled(id, h.now),
		"it was handed back, so it waits before the next time")
	for _, d := range h.decisions {
		assert.NotEqual(t, incident.Resolve, d.Action,
			"the workload never recovered, so nothing resolved")
	}
}

// A covered workload is left alone: the backstop stays quiet when
// nothing was lost.
func TestCoverageCheckLeavesCoveredWorkloadsAlone(t *testing.T) {
	h, id := crashingHarness(t)
	h.run(h.now.Add(40*time.Minute), 10*time.Second)
	assert.True(t, h.engine.coverage.Settled(id, h.now),
		"nothing was handed back")
}
