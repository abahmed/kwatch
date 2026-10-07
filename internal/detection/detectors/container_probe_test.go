package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

type probeFixture struct {
	message  string
	count    int
	ready    bool
	age      time.Duration
	started  time.Duration
	budget   float64
	deleting bool
}

func detectProbe(t *testing.T, f probeFixture) []detection.Finding {
	t.Helper()
	model := newTestModel()
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	observeEntity(model, pod, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase:    inventory.Text("Running"),
			kube.AttrDeleting: inventory.Bool(f.deleting),
		})
	id := kube.ContainerID("default", "app", "main")
	attrs := map[string]inventory.Value{
		kube.AttrState: inventory.Text("running"),
		kube.AttrReady: inventory.Bool(f.ready),
		kube.AttrStartedAt: inventory.Time(
			podNodeNow.Add(-f.started)),
	}
	if f.budget > 0 {
		attrs[kube.AttrProbeBudget] = inventory.Number(f.budget)
	}
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute), attrs)
	relateEntity(model, id, inventory.PartOf, pod)
	if f.message != "" {
		noteEntity(model, id, "Unhealthy", f.message, f.count,
			podNodeNow.Add(-f.age))
	}
	return classified(Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
}

func TestContainerProbeFailures(t *testing.T) {
	cases := []struct {
		name    string
		fixture probeFixture
		reason  string
		mode    string
	}{
		{"liveness repeated", probeFixture{
			message: "Liveness probe failed: HTTP probe failed with " +
				"statuscode: 500", count: 3, ready: true,
			started: time.Hour,
		}, reasons.LivenessProbeFailed, "Probe.Liveness"},
		{"readiness sustained unready", probeFixture{
			message: "Readiness probe failed: connection refused",
			count:   1, started: time.Hour,
		}, reasons.ReadinessProbeFailed, "Probe.Readiness"},
		{"startup errored", probeFixture{
			message: "Startup probe errored and resulted in unknown " +
				"state: timeout", count: 4, started: time.Hour, budget: 60,
		}, reasons.StartupProbeFailed, "Probe.Startup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := detectProbe(t, tc.fixture)
			require.Len(t, found, 1)
			assert.Equal(t, tc.reason, found[0].Reason)
			assert.Equal(t, tc.mode, string(found[0].Mode))
			assert.Equal(t, detection.Degraded, found[0].Health)
			assert.Equal(t, "probe", found[0].Evidence[0].Label)
			assert.Equal(t, tc.fixture.message, found[0].Evidence[0].Value)
		})
	}
}

func TestContainerProbeFailuresIgnored(t *testing.T) {
	cases := []struct {
		name    string
		fixture probeFixture
	}{
		{"no events", probeFixture{started: time.Hour}},
		{"single liveness failure while ready", probeFixture{
			message: "Liveness probe failed: timeout", count: 1,
			ready: true, started: time.Hour,
		}},
		{"readiness failing but ready again", probeFixture{
			message: "Readiness probe failed: 503", count: 9, ready: true,
			started: time.Hour,
		}},
		{"readiness within startup budget", probeFixture{
			message: "Readiness probe failed: 503", count: 5,
			started: 20 * time.Second, budget: 120,
		}},
		{"stale failures", probeFixture{
			message: "Liveness probe failed: timeout", count: 5,
			age: 10 * time.Minute, started: time.Hour,
		}},
		{"pod shutting down", probeFixture{
			message: "Readiness probe failed: 503", count: 5,
			started: time.Hour, deleting: true,
		}},
		{"other message", probeFixture{
			message: "Probe warning", count: 5, started: time.Hour,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, detectProbe(t, tc.fixture))
		})
	}
}

// The kubelet's event spam filter slows a steady failure to one event
// per five minutes, so the newest event can be older than probeRecency
// while the container is still not ready. The finding must stay.
func TestReadinessFindingStaysWhileStillNotReady(t *testing.T) {
	found := detectProbe(t, probeFixture{
		message: "Readiness probe failed: connection refused",
		count:   1, age: 10 * time.Minute, started: time.Hour,
	})

	require.Len(t, found, 1)
	assert.Equal(t, reasons.ReadinessProbeFailed, found[0].Reason)
}

func TestReadinessFindingEndsOnceTheEventIsVeryOld(t *testing.T) {
	assert.Empty(t, detectProbe(t, probeFixture{
		message: "Readiness probe failed: connection refused",
		count:   1, age: time.Hour, started: 2 * time.Hour,
	}))
}

// detectThrottledProbe returns the liveness finding of a container that
// the stats poller saw throttled pct of the time, with a 200m limit.
func detectThrottledProbe(
	t *testing.T, message string, pct float64,
) detection.Finding {
	t.Helper()
	model := newTestModel()
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	observeEntity(model, pod, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
		})
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute),
		map[string]inventory.Value{
			kube.AttrState:        inventory.Text("running"),
			kube.AttrReady:        inventory.Bool(true),
			kube.AttrStartedAt:    inventory.Time(podNodeNow.Add(-time.Hour)),
			kube.AttrThrottledPct: inventory.Number(pct),
			kube.AttrCPULimit:     inventory.Number(200),
		})
	relateEntity(model, id, inventory.PartOf, pod)
	noteEntity(model, id, "Unhealthy", message, 3,
		podNodeNow.Add(-time.Minute))
	found := Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id))
	for _, f := range found {
		if f.Reason == reasons.LivenessProbeFailed {
			return f
		}
	}
	t.Fatalf("no liveness finding in %v", found)
	return detection.Finding{}
}

func TestProbeFailureShowsCPUThrottling(t *testing.T) {
	f := detectThrottledProbe(t, "Liveness probe failed: Get "+
		`"http://10.0.0.1:8080/healthz": context deadline exceeded`, 72.4)
	assert.Equal(t, "72%", evidenceOf(f, detection.EvidenceCPUThrottled))
	assert.Equal(t, "200m", evidenceOf(f, detection.EvidenceCPULimit))
}

func TestProbeFailureIgnoresLightThrottling(t *testing.T) {
	f := detectThrottledProbe(t, "Liveness probe failed: timeout", 12)
	assert.Empty(t, evidenceOf(f, detection.EvidenceCPUThrottled))
	assert.Empty(t, evidenceOf(f, detection.EvidenceCPULimit))
}

func evidenceOf(f detection.Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}
