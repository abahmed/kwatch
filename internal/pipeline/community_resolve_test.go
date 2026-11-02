package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestOpenAnnouncedKeepsOnlyWhatSomeoneWasToldAndIsOpen(t *testing.T) {
	at := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	told := func(id string, state incident.State) incident.Record {
		return incident.Record{ID: id, State: state, Announced: at}
	}
	unheard := told("unheard", incident.Open)
	unheard.Unheard = true
	records := []incident.Record{
		told("open", incident.Open),
		told("recovering", incident.Recovering),
		told("resolved", incident.Resolved),
		told("flapping", incident.Flapping),
		told("baselined", incident.Open),
		unheard,
		{ID: "settling", State: incident.Settling},
	}

	got := openAnnounced(records, func(id string) bool {
		return id == "baselined"
	})

	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []string{"open", "recovering", "flapping"}, ids)
}

// communityHarness is an engine in the Community edition over store.
func communityHarness(
	t *testing.T, at time.Time, store *memStore,
) *harness {
	t.Helper()
	h := newHarnessWith(t, at, func(d *Dependencies) {
		d.Entitlements = feature.CommunityEntitlements()
		d.Store = store
	})
	require.NoError(t, h.engine.restore())
	return h
}

// A failure that begins while Community runs is announced, saved, and
// resolved by the next run once it is gone: otherwise a pager alert stays
// open for good after a restart. A failure that was already there when
// kwatch started was never announced, and is never saved or resolved.
func TestCommunityResolvesAnIncidentItAnnouncedBeforeARestart(t *testing.T) {
	saved, now := announcedBeforeRestart(t)
	assert.Equal(t, 1, resolvesAfterRestart(t, saved, now))
}

// A Pro run whose licence lapsed restarts as a baseline. An incident it
// announced that was flapping is restored like an open one, so its
// resolve still closes the thread and the alert.
func TestCommunityResolvesAFlappingIncidentAfterARestart(t *testing.T) {
	saved, now := announcedBeforeRestart(t)
	saved[0].State = incident.Flapping
	saved[0].RecoveringSince = now
	kept := openAnnounced(saved, nil)
	require.Len(t, kept, 1, "the flapping incident is kept")
	assert.Equal(t, 1, resolvesAfterRestart(t, kept, now))
}

// announcedBeforeRestart runs a Community engine over a baselined failure
// and a new one, and returns what it saves and the time it stopped.
func announcedBeforeRestart(t *testing.T) ([]incident.Record, time.Time) {
	t.Helper()
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	first := communityHarness(t, start, &memStore{})
	first.add(kube.NodeSchema{}, node("n1", time.Time{}))
	for _, name := range []string{"queue", "web"} {
		d, rs := deployment(name)
		d.Status.ReadyReplicas = *d.Spec.Replicas
		first.add(kube.DeploymentSchema(), d)
		first.add(kube.ReplicaSetSchema(), rs)
		if name == "queue" {
			first.add(kube.PodSchema{}, crashingPod(
				"queue-a", rs.Name, "n1", start))
		}
	}
	first.engine.reconcileDowntime()
	first.run(start.Add(5*time.Minute), 10*time.Second)
	require.Empty(t, first.messages, "the existing failure is baselined")
	_, webRS := deployment("web")
	first.add(kube.PodSchema{}, crashingPod("web-a", webRS.Name, "n1",
		first.now))
	first.run(first.now.Add(3*time.Minute), 10*time.Second)
	require.NotEmpty(t, first.messages, "the new failure is announced")

	saved := openAnnounced(first.engine.deps.Incidents.Export(),
		first.engine.announcer.collect.Startup.Baselined)
	require.Len(t, saved, 1, "only the announced incident is saved")
	return saved, first.now
}

// resolvesAfterRestart restarts a Community engine over saved an hour
// after stopped, with the failure gone, and counts the resolves it sends.
func resolvesAfterRestart(
	t *testing.T, saved []incident.Record, stopped time.Time,
) int {
	t.Helper()
	later := stopped.Add(time.Hour)
	second := communityHarness(t, later, &memStore{records: saved})
	require.Len(t, second.engine.deps.Incidents.Incidents(), 1,
		"the announced incident comes back")
	second.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("web")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	second.add(kube.DeploymentSchema(), d)
	second.add(kube.ReplicaSetSchema(), rs)
	second.engine.reconcileDowntime()
	second.run(later.Add(time.Hour), 10*time.Second)

	var resolved int
	for _, m := range second.messages {
		if m.Status == notification.StatusResolved {
			resolved++
		}
	}
	return resolved
}
