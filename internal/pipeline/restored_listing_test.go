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
	return restartedHarnessWith(t, nil)
}

// restartedHarnessWith is restartedHarness with more dependencies set
// by with, when not nil.
func restartedHarnessWith(
	t *testing.T, with func(*Dependencies),
) *harness {
	t.Helper()
	old, _ := crashingHarness(t)
	store := &memStore{
		records: old.engine.deps.Incidents.Export(),
		startup: &announce.StartupState{Complete: true},
	}
	h := newHarnessWith(t, old.now, func(d *Dependencies) {
		d.Store = store
		if with != nil {
			with(d)
		}
	})
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

// A restart announces no restored incident by itself, and a short
// restart that finds the incidents told before and unchanged says
// nothing at all (the audit log alone records it).
func TestRestartListsRestoredIncidentsStillFailing(t *testing.T) {
	h := restartedHarness(t)
	h.run(h.now.Add(restoreGrace+time.Minute), 10*time.Second)
	h.run(h.now.Add(time.Hour), 30*time.Second)

	assert.Empty(t, restoredSummaries(h),
		"told before and unchanged: nothing delivered")
	for _, d := range h.decisions {
		assert.NotEqual(t, incident.Announce, d.Action,
			"no individual message for a restored incident")
	}
}

// After a long downtime the gap report waits for the restored
// incidents and goes out once, as the one message of the restart, with
// the count of the ongoing ones it does not list again.
func TestRestartSendsTheGapWithTheRestoredCount(t *testing.T) {
	var notified []string
	h := restartedHarnessWith(t, func(d *Dependencies) {
		d.Gap = Gap{Start: d.Clock.Now().Add(-time.Hour),
			End: d.Clock.Now().Add(-time.Minute)}
		d.Notify = func(text string) { notified = append(notified, text) }
	})
	h.engine.reconcileDowntime()
	assert.Empty(t, notified, "the gap waits for the restored incidents")
	h.run(h.now.Add(restoreGrace+time.Minute), 10*time.Second)
	h.run(h.now.Add(time.Hour), 30*time.Second)

	require.Len(t, notified, 1)
	assert.Contains(t, notified[0], "kwatch was not running from")
	assert.True(t, strings.HasSuffix(notified[0],
		"\n\n1 ongoing issue unchanged."), notified[0])
	for _, m := range restoredSummaries(h) {
		assert.NotEmpty(t, m.Carrier, "nothing delivered beside it")
	}
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
