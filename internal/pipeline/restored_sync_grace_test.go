package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// A slow sync must not use up the restore grace: the model is rebuilt
// only once the caches synced, so the grace runs from the sync. Once it
// is rebuilt, though, a restored incident whose findings did not come
// back recovers then, not at the 10-minute upper bound.
func TestRestoreGraceRunsFromTheSourceSync(t *testing.T) {
	old, _ := crashingHarness(t)
	store := &memStore{
		records: old.engine.deps.Incidents.Export(),
		startup: &announce.StartupState{Complete: true},
	}
	start := old.now.Add(8 * time.Hour)
	h := newHarnessWith(t, start, func(d *Dependencies) { d.Store = store })
	require.NoError(t, h.engine.restore())

	// The caches sync late, 9 minutes in; the workload looks healthy.
	h.run(start.Add(9*time.Minute), 10*time.Second)
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, pod("payments-1", rs.Name, "n1", true, h.now))
	synced := h.now
	h.engine.reconcileDowntime()

	// Past the bound the restore set, inside the settle after the sync.
	h.run(start.Add(restoreGrace+30*time.Second), 10*time.Second)
	for _, p := range h.engine.Incidents() {
		assert.Equal(t, incident.Open, p.State,
			"the model is not rebuilt yet")
	}

	// Rebuilt: it recovers and resolves one hold later, well before
	// the bound of 10 minutes from the sync.
	h.run(synced.Add(restoreSettle+incident.DefaultHold+
		30*time.Second), 10*time.Second)
	require.Less(t, restoreSettle+incident.DefaultHold, restoreGrace)
	for _, p := range h.engine.Incidents() {
		assert.Equal(t, incident.Resolved, p.State,
			"nothing came back once the model was rebuilt")
	}
}
