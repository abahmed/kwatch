package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

func TestBaselineStartFollowsTheDiskStateEntitlement(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    feature.Entitlements
		want bool
	}{
		{"community", feature.CommunityEntitlements(), true},
		{"pro", feature.ProEntitlements(), false},
		{"unset", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
				func(d *Dependencies) {
					d.Entitlements, d.Store = tc.e, &memStore{}
				})
			assert.Equal(t, tc.want,
				e.announcer.collect.Startup.Baseline)
			assert.NotNil(t, e.storage.incidents)
			assert.Equal(t, tc.want, e.storage.openOnly,
				"a baseline saves only the announced open incidents")
		})
	}
}

// stateOfALongRun is a store that holds the incident of an earlier run.
func stateOfALongRun(t *testing.T, start time.Time) *memStore {
	t.Helper()
	earlier, _ := coldHarness(t, start)
	earlier.run(start.Add(90*time.Second), 10*time.Second)
	records := earlier.engine.deps.Incidents.Export()
	require.Len(t, records, 1)
	return &memStore{records: records,
		prints: map[string]string{"pod/shop/queue-a": "v1"}}
}

func TestProRestoresTheIncidentsOfTheEarlierRun(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	store := stateOfALongRun(t, start)
	h := newHarnessWith(t, start.Add(time.Hour), func(d *Dependencies) {
		d.Store = store
		d.Entitlements = feature.ProEntitlements()
	})

	require.NoError(t, h.engine.restore())

	assert.Len(t, h.engine.deps.Incidents.Incidents(), 1)
	assert.NotNil(t, h.engine.storage.incidents)
}

func TestCommunityStartsFromABaselineAndRestoresNothing(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	store := stateOfALongRun(t, start)
	h := newHarnessWith(t, start.Add(time.Hour), func(d *Dependencies) {
		d.Store = store
		d.Entitlements = feature.CommunityEntitlements()
	})

	require.NoError(t, h.engine.restore())

	assert.Empty(t, h.engine.deps.Incidents.Incidents(),
		"no persistent incident restore")
	startup := h.engine.announcer.collect.Startup
	assert.True(t, startup.ColdStart)
	assert.True(t, startup.AwaitSync, "existing failures are held")
	assert.True(t, h.engine.storage.openOnly,
		"only announced open incidents are saved")
	assert.Empty(t, h.engine.storage.saved, "no fingerprints are loaded")
}

func messageKeys(h *harness) []string {
	var keys []string
	for _, m := range h.messages {
		keys = append(keys, m.Key)
	}
	return keys
}

// communityStart is a Community engine whose queue workload is already
// crashing when it starts.
func communityStart(t *testing.T, start time.Time) (*harness, *corev1.Pod) {
	t.Helper()
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Entitlements = feature.CommunityEntitlements()
	})
	require.NoError(t, h.engine.restore())
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("queue")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	crashing := crashingPod("queue-a", rs.Name, "n1", start)
	h.add(kube.PodSchema{}, crashing)
	h.engine.reconcileDowntime()
	return h, crashing
}

// heal makes the crashing pod healthy at the harness's time.
func heal(h *harness, crashing *corev1.Pod) {
	healthy := pod("queue-a", crashing.OwnerReferences[0].Name, "n1",
		true, h.now)
	healthy.ResourceVersion = "next"
	h.engine.Submit(context.Background(), kube.NewTranslator(
		kube.PodSchema{}).Updated(crashing, healthy, h.now)...)
}

// announcements are the announcements h sent, resolves left out.
func announcements(h *harness) []incident.Decision {
	var out []incident.Decision
	for i, decision := range h.decisions {
		if decision.Action == incident.Announce &&
			h.messages[i].Status != notification.StatusResolved {
			out = append(out, decision)
		}
	}
	return out
}

// A failure that exists when Community starts is held while the startup
// window is open, then announced on its own, once, as a new failure is:
// no startup summary. From then on it is a normal incident.
func TestCommunityAnnouncesAFailureStillFailingAfterTheWindow(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, crashing := communityStart(t, start)

	h.run(start.Add(announce.StartupWindow-10*time.Second),
		10*time.Second)
	require.Len(t, h.engine.deps.Incidents.Incidents(), 1)
	assert.Empty(t, h.messages, "held while the window is open: %v",
		messageKeys(h))

	h.run(start.Add(5*time.Minute), 10*time.Second)
	sent := announcements(h)
	require.Len(t, sent, 1, "announced once after the window")
	for _, m := range h.messages {
		assert.NotContains(t, m.Note, "already",
			"a single announcement, not a startup summary")
	}
	heal(h, crashing)
	h.run(h.now.Add(10*time.Minute), 10*time.Second)
	last := h.messages[len(h.messages)-1]
	assert.Equal(t, notification.StatusResolved, last.Status,
		"its recovery is announced too")
	assert.Len(t, announcements(h), 1)
}

// A failure that exists at start and recovers inside the startup window
// is never heard of, not when it fails and not when it resolves.
func TestCommunityStaysSilentAboutAFailureGoneInTheWindow(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, crashing := communityStart(t, start)

	h.run(start.Add(90*time.Second), 10*time.Second)
	heal(h, crashing)
	h.run(start.Add(15*time.Minute), 10*time.Second)
	assert.Empty(t, h.messages, "nothing about it: %v", messageKeys(h))

	d2, rs2 := deployment("web")
	d2.Status.ReadyReplicas = *d2.Spec.Replicas
	h.add(kube.DeploymentSchema(), d2)
	h.add(kube.ReplicaSetSchema(), rs2)
	h.add(kube.PodSchema{}, crashingPod("web-a", rs2.Name, "n1", h.now))
	h.run(h.now.Add(3*time.Minute), 10*time.Second)
	require.NotEmpty(t, announcements(h), "a new failure alerts")
}

func TestCommunityDoesNotReportChangesMadeWhileDown(t *testing.T) {
	h := newHarnessWith(t, time.Now(), func(d *Dependencies) {
		d.Entitlements = feature.CommunityEntitlements()
	})
	assert.Nil(t, h.engine.applyDowntime(map[string]string{"a": "b"}))
}

func TestStatusWithoutEntitlementsIsCommunity(t *testing.T) {
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Entitlements = nil })
	report := e.Status()
	assert.Equal(t, "Community", report.Edition)
	assert.True(t, report.Brief)
	assert.False(t, e.deps.entitled(feature.Flapping))
	assert.True(t, e.deps.entitled(feature.PodDetection))
}

func TestStatusCarriesTheEditionNotice(t *testing.T) {
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
		func(d *Dependencies) {
			d.ProDetectorsOff = []string{"workloads", "storage"}
			d.CommunitySends = "slack (inactive: pagerduty)"
		})

	report := e.Status()

	assert.Equal(t, []string{"workloads", "storage"}, report.ProDetectorsOff)
	assert.Equal(t, "slack (inactive: pagerduty)", report.CommunitySends)
	assert.Contains(t, report.Text(), "Pro detectors not running: 2")
}
