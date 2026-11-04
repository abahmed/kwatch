package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A liveness probe that fails after the container crashed by itself
// does not lead the sentence: the crash does. When the kubelet killed
// the container for the probe, the probe does.
func TestSummaryWhatLeadsWithTheCrashNotItsLivenessSymptom(t *testing.T) {
	p := summaryIncident()
	d := toDetail(p)
	code := 1
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	d.Probes = []ProbeRow{{Container: "api", Probe: "liveness",
		LastMessage: "connect: connection refused", LastAt: at}}
	d.Containers = []ContainerRow{{Container: "api", Restarts: 5,
		LastReason: "Error", LastExitCode: &code, LastFinished: at}}

	assert.Equal(t, "Deployment api in shop keeps crashing (5 restarts).",
		whatOf(&d, p))

	killed := 137
	d.Containers[0].LastExitCode = &killed
	d.Warnings = []RecentWarning{{Reason: "Killing", Object: "Pod/api-1",
		Message: "Container api failed liveness probe, will be restarted"}}
	assert.Equal(t, "Deployment api in shop is failing its liveness "+
		"check and being restarted (5 restarts).", whatOf(&d, p))
}

// crashOnLivenessContainer puts the first container row of d on the pod
// of its liveness probe row, its last run ending as the probe failed
// with a refused connection: nothing listened after the crash.
func crashOnLivenessContainer(d *IncidentDetail) {
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for i, p := range d.Probes {
		if p.Probe == "liveness" {
			d.Probes[i].Container, d.Probes[i].LastAt = "api", at
			d.Probes[i].LastMessage = "Liveness probe failed: " +
				"connection refused"
			d.Containers[0].Pod, d.Containers[0].LastFinished = p.Pod, at
		}
	}
}

// crashDetail is an incident whose liveness probe on api-1/api fails
// at a time and whose container row ended its last run with code and
// reason.
func crashDetail(code int, reason string) (IncidentDetail, time.Time) {
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	d := toDetail(summaryIncident())
	d.Probes = []ProbeRow{{Pod: "api-1", Container: "api",
		Probe: "liveness", LastAt: at,
		LastMessage: "Liveness probe failed: connection refused"}}
	d.Containers = []ContainerRow{{Pod: "api-1", Container: "api",
		Restarts: 5, LastReason: reason, LastExitCode: &code,
		LastFinished: at}}
	return d, at
}

// A stop by signal or a memory kill is not a crash of its own, as the
// detectors judge it: the liveness failure is not hidden behind it.
func TestCrashedOnItsOwnLeavesOutKillsAndOOM(t *testing.T) {
	for _, c := range []struct {
		code   int
		reason string
		want   bool
	}{
		{1, "Error", true}, {137, "Error", false}, {143, "Error", false},
		{1, "OOMKilled", false},
	} {
		d, _ := crashDetail(c.code, c.reason)
		assert.Equal(t, c.want, crashedOnItsOwn(&d), "%d %s", c.code,
			c.reason)
	}
}

// Only the crash of the probe's own container, at most the lead before
// the failure, makes the failure a symptom.
func TestCrashedOnItsOwnNeedsTheProbesContainerAtTheFailure(
	t *testing.T,
) {
	d, at := crashDetail(1, "Error")
	d.Containers[0].Container = "sidecar"
	assert.False(t, crashedOnItsOwn(&d), "another container")

	d, _ = crashDetail(1, "Error")
	d.Containers[0].Pod = "api-2"
	assert.False(t, crashedOnItsOwn(&d), "another pod")

	d, _ = crashDetail(1, "Error")
	d.Containers[0].LastFinished = at.Add(-kube.LivenessKillLead -
		time.Second)
	assert.False(t, crashedOnItsOwn(&d), "an old crash")

	d, _ = crashDetail(1, "Error")
	d.Containers[0].LastFinished = at.Add(-kube.LivenessKillLead)
	assert.True(t, crashedOnItsOwn(&d), "a crash within the lead")
}
