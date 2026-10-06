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

// After a long monitoring gap, an incident whose workload is healthy
// when kwatch starts resolves one hold after the restore grace, not
// hours later.
func TestRestoredIncidentResolvesPromptlyAfterGap(t *testing.T) {
	old, _ := crashingHarness(t)
	store := &memStore{
		records: old.engine.deps.Incidents.Export(),
		startup: &announce.StartupState{Complete: true},
	}
	start := old.now.Add(8 * time.Hour)
	h := newHarnessWith(t, start, func(d *Dependencies) { d.Store = store })
	require.NoError(t, h.engine.restore())
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 2, 2
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, pod("payments-1", rs.Name, "n1", true, start))

	resolved := func() bool {
		for _, decision := range h.decisions {
			if decision.Action == incident.Resolve {
				return true
			}
		}
		return false
	}
	require.True(t, h.runUntil(resolved, start.Add(2*time.Hour),
		10*time.Second), "the incident resolves")
	assert.WithinDuration(t, start.Add(restoreGrace+incident.DefaultHold),
		h.now, 2*time.Minute)
}
