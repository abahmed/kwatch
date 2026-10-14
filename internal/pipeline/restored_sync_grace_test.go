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

// A slow sync must not use up the restore grace: findings gated by an
// observation-time onset (a sustained cpu, a hook) only re-raise one
// sustain after the caches synced, so the grace runs from the sync.
func TestRestoreGraceRunsFromTheSourceSync(t *testing.T) {
	old, _ := crashingHarness(t)
	store := &memStore{
		records: old.engine.deps.Incidents.Export(),
		startup: &announce.StartupState{Complete: true},
	}
	start := old.now.Add(8 * time.Hour)
	h := newHarnessWith(t, start, func(d *Dependencies) { d.Store = store })
	require.NoError(t, h.engine.restore())

	// The caches sync late, 8 minutes in; the workload looks healthy.
	h.run(start.Add(8*time.Minute), 10*time.Second)
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, pod("payments-1", rs.Name, "n1", true, h.now))
	h.engine.reconcileDowntime()

	// Past the grace the restore set, before the extended one ends.
	h.run(start.Add(restoreGrace+3*time.Minute), 10*time.Second)
	for _, p := range h.engine.Incidents() {
		assert.Equal(t, incident.Open, p.State,
			"the incident waits for the findings to re-raise")
	}
}
