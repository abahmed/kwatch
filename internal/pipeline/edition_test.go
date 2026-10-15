package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
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
			assert.Equal(t, tc.want, e.storage.incidents == nil,
				"no incident writer in a baseline")
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
	assert.Nil(t, h.engine.storage.incidents, "incidents are not saved")
	assert.Empty(t, h.engine.storage.saved, "no fingerprints are loaded")
}

func messageKeys(h *harness) []string {
	var keys []string
	for _, m := range h.messages {
		keys = append(keys, m.Key)
	}
	return keys
}

// A failure that exists when kwatch starts is the baseline: nobody is
// told about it, not when it starts, changes or ends. A failure that
// begins later is announced.
func TestCommunityOnlyAnnouncesFailuresThatBeginAfterStart(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
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

	h.run(start.Add(5*time.Minute), 10*time.Second)
	existing := h.engine.deps.Incidents.Incidents()
	require.Len(t, existing, 1, "the incident is tracked")
	assert.Empty(t, h.messages,
		"an existing failure is baselined, not announced: %v",
		messageKeys(h))

	healthy := pod("queue-a", rs.Name, "n1", true, h.now)
	healthy.ResourceVersion = "next"
	h.engine.Submit(context.Background(), kube.NewTranslator(
		kube.PodSchema{}).Updated(crashing, healthy, h.now)...)
	h.run(h.now.Add(10*time.Minute), 10*time.Second)
	assert.Empty(t, h.messages, "its recovery is silent too")

	d2, rs2 := deployment("web")
	d2.Status.ReadyReplicas = *d2.Spec.Replicas
	h.add(kube.DeploymentSchema(), d2)
	h.add(kube.ReplicaSetSchema(), rs2)
	h.add(kube.PodSchema{}, crashingPod("web-a", rs2.Name, "n1", h.now))
	h.run(h.now.Add(3*time.Minute), 10*time.Second)

	var announced []incident.Decision
	for i, decision := range h.decisions {
		if decision.Action == incident.Announce &&
			h.messages[i].Status != notification.StatusResolved {
			announced = append(announced, decision)
		}
	}
	require.NotEmpty(t, announced, "a new failure alerts")
	assert.NotEqual(t, existing[0].ID, announced[0].Incident.ID)
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
