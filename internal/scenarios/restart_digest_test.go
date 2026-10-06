package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

const restartCrashMessage = "panic: sync: batch window is closed"

// crashBoth puts both pods of the workload into a crash loop for d.
func crashBoth(c *cluster, w *workload, d time.Duration, restarts int32) {
	for i := range 2 {
		c.update(w.pod(i, "n1", crashLoop(1, "Error",
			restartCrashMessage, restarts)))
	}
	w.setReady(0)
	c.update(w.objects())
	c.after(d)
}

// restartCrashFirstSession is kwatch watching a known 2-replica
// Deployment: it crashed on each of two earlier days and people heard of
// both, so its third-day crash loop is routine boot noise and only the
// digest carries it. The session ends four minutes into that third day.
func restartCrashFirstSession() (replay.Log, restartFailing) {
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "sync", "registry.example.com/sync:7", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	c.after(time.Minute)
	for day, length := range []time.Duration{
		12 * time.Minute, 12 * time.Minute,
	} {
		crashBoth(c, w, length, 6)
		for i := range 2 {
			c.update(w.pod(i, "n1", startedNow))
		}
		w.setReady(2)
		c.update(w.objects())
		c.after(23*time.Hour + time.Duration(day)*time.Minute)
	}
	crashBoth(c, w, 4*time.Minute, 6)
	return c.log(), restartFailing{cluster: c, w: w}
}

// restartCrashSecondSession is the restart: the live objects are listed
// again, and the pods keep crash-looping for the next 40 minutes, well
// past the boot window, with nothing ready.
func restartCrashSecondSession(
	f restartFailing, restartAt time.Time,
) replay.Log {
	c := newCluster(scenarioStart, "")
	c.now = restartAt
	c.list(remaining(f.cluster)...)
	w := f.w
	w.c = c
	for restarts := int32(7); restarts <= 13; restarts++ {
		c.after(5 * time.Minute)
		crashBoth(c, w, 0, restarts)
	}
	c.after(5 * time.Minute)
	log := c.log()
	log.Start = restartAt.UTC()
	return log
}

// A restart in the middle of a routine crash loop must not make it
// routine for good. The restored digest-only incident is still the same
// failure: once it has lasted the boot window with nothing ready, it
// reaches the notify tier as a message of its own.
func TestRestartMidCrashDigestReachesNotify(t *testing.T) {
	store := newPersistingStore()
	first, failing := restartCrashFirstSession()
	firstRun := restartReplay(t, first, store, replay.Options{
		SyncAt: first.Start, Tail: time.Minute,
	})
	require.NotZero(t, store.records(), "the first session saved state")
	for _, p := range firstRun.Incidents {
		require.Equal(t, incident.Digest, p.Tier, "routine in session 1")
	}

	restartAt := firstRun.End.Add(restartGap)
	second := restartCrashSecondSession(failing, restartAt)
	secondRun := restartReplay(t, second, store, replay.Options{
		SyncAt: restartAt, Tail: 20 * time.Minute,
	})

	var loud []incident.Decision
	for _, d := range secondRun.Decisions {
		if d.Incident.Tier >= incident.Notify {
			loud = append(loud, d)
		}
	}
	require.NotEmpty(t, loud, "it must reach notify: %s",
		describeDecisions(secondRun))
	assert.Equal(t, kube.KindDeployment, loud[0].Incident.Root.Kind)
	assert.Equal(t, 1, len(decisionsOf(secondRun, incident.Announce, "")),
		"announced once: %s", describeDecisions(secondRun))
	for _, m := range secondRun.Messages {
		assert.NotContains(t, m.Title, "restarted and",
			"the restored incident spoke for itself; listing it again "+
				"would be a second message about the same failure")
	}
}
