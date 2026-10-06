package pipeline

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// restartedHarness restarts kwatch (a new engine on the stored incidents)
// while the crash loop of crashingHarness goes on.
func restartedHarness(t *testing.T) *harness {
	t.Helper()
	old, _ := crashingHarness(t)
	store := &memStore{
		records: old.engine.deps.Incidents.Export(),
		startup: &announce.StartupState{Complete: true},
	}
	h := newHarnessWith(t, old.now, func(d *Dependencies) { d.Store = store })
	require.NoError(t, h.engine.restore())
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("payments")
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = 0, 0
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, crashingPod("payments-1", rs.Name, "n1",
		old.now.Add(-time.Hour)))
	return h
}

func restoredSummaries(h *harness) []notification.Message {
	var out []notification.Message
	for i, d := range h.decisions {
		if d.Reason == "restored incidents" {
			out = append(out, h.messages[i])
		}
	}
	return out
}

// A restart announces no restored incident by itself; one message lists
// the ones that still fail once their findings are back.
func TestRestartListsRestoredIncidentsStillFailing(t *testing.T) {
	h := restartedHarness(t)
	h.run(h.now.Add(restoreGrace+time.Minute), 10*time.Second)

	listed := restoredSummaries(h)
	require.Len(t, listed, 1)
	assert.True(t, strings.HasPrefix(listed[0].Key,
		notification.SummaryKeyPrefix))
	assert.Contains(t, listed[0].Note, "payments")
	for _, d := range h.decisions {
		assert.NotEqual(t, incident.Announce, d.Action,
			"no individual message for a restored incident")
	}

	h.run(h.now.Add(time.Hour), 30*time.Second)
	assert.Len(t, restoredSummaries(h), 1, "listed once per restart")
}

// Nothing to list when the restored incident is gone by the end of the
// grace: the workload recovered meanwhile.
func TestRestartListsNothingForRecoveredIncidents(t *testing.T) {
	h := restartedHarness(t)
	d, rs := deployment("payments")
	healthy := d.DeepCopy()
	healthy.Status.ReadyReplicas, healthy.Status.AvailableReplicas = 2, 2
	h.engine.Submit(ctxBackground(), kube.NewTranslator(
		kube.DeploymentSchema()).Updated(d, healthy, h.now)...)
	h.add(kube.PodSchema{}, pod("payments-1", rs.Name, "n1", true, h.now))
	h.run(h.now.Add(restoreGrace+time.Minute), 10*time.Second)
	assert.Empty(t, restoredSummaries(h))
}
