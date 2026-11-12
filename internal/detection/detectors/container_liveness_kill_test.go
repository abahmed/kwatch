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

// A process that ignores SIGTERM is killed when its grace period ends:
// the liveness kill then ends with 137, not with a memory kill.
func TestLivenessKillThatEndsInSIGKILLIsALivenessKill(t *testing.T) {
	found := detectKill(t, killFixture{state: "running", restarts: 4,
		exit: 137, finished: 20 * time.Second, message: livenessMessage})

	require.Len(t, found, 1)
	assert.Equal(t, reasons.LivenessKilled, found[0].Reason)
	assert.Equal(t, "HTTP 500",
		evidenceOf(found[0], detection.EvidenceProbeResult))
	assert.Equal(t, "3", evidenceOf(found[0], "failures"))
}

// A container that exits on its own with an error refuses its liveness
// probe while it is down: the probe is the crash's symptom.
func TestLivenessFailureOfAContainerThatCrashedIsLeftToTheCrash(
	t *testing.T,
) {
	found := detectKill(t, killFixture{state: "running", restarts: 4,
		exit: 1, finished: 20 * time.Second,
		message: "Liveness probe failed: Get \"http://10.0.0.9:8080/" +
			"healthz\": dial tcp 10.0.0.9:8080: connect: connection " +
			"refused"})

	require.NotEmpty(t, found)
	for _, f := range found {
		assert.NotEqual(t, reasons.LivenessKilled, f.Reason)
		assert.NotContains(t, f.Summary, "liveness")
	}
	assert.Contains(t, found[0].Summary,
		"keeps exiting on its own with an error (exit code 1)")
	assert.Equal(t, "1",
		evidenceOf(found[0], detection.EvidenceExitedOnOwn))
}

func TestReadinessAnsweredWithAnErrorStatusSaysItsOwnCheckFails(
	t *testing.T,
) {
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute),
		map[string]inventory.Value{
			kube.AttrState:     inventory.Text("running"),
			kube.AttrReady:     inventory.Bool(false),
			kube.AttrStartedAt: inventory.Time(podNodeNow.Add(-time.Hour)),
		})
	noteEntity(model, id, "Unhealthy", "Readiness probe failed: HTTP "+
		"probe failed with statuscode: 500", 5, podNodeNow.Add(-time.Minute))

	found := Container{}.Detect(testDetectorContext(model, podNodeNow),
		entityOf(model, id))

	require.Len(t, found, 1)
	assert.Equal(t, reasons.ReadinessProbeFailed, found[0].Reason)
	assert.Equal(t, "Container keeps failing its own readiness check "+
		"(HTTP 500) while it runs", found[0].Summary)
}

func TestProbeResultNamesHowAProbeFailed(t *testing.T) {
	cases := map[string]string{
		"HTTP probe failed with statuscode: 503": "HTTP 503",
		"context deadline exceeded":              "timed out",
		"dial tcp 10.0.0.1:80: connect: connection refused": "connection " +
			"refused",
		"read: connection reset by peer": "connection reset",
		"exec failed: exit 1":            "",
	}
	for message, want := range cases {
		assert.Equal(t, want, probeResult(message), message)
	}
}

// A program may exit 1 when the kubelet stops it for its liveness
// probe: the kubelet's own note at the end of the run makes it a
// liveness kill, not a crash of its own.
func TestLivenessKillThatEndsInAnErrorCodeIsALivenessKill(t *testing.T) {
	f := killFixture{state: "running", restarts: 4, exit: 1,
		finished: 20 * time.Second, message: livenessMessage}
	f.stop = "Container main failed liveness probe, will be restarted"
	f.stopAt = podNodeNow.Add(-25 * time.Second)
	found := detectKill(t, f)

	require.Len(t, found, 1)
	assert.Equal(t, reasons.LivenessKilled, found[0].Reason)
	assert.NotContains(t, found[0].Summary, "exiting on its own")
}

// A liveness stop long before the last run ended explained an earlier
// run, not this one: the error exit is the program's own.
func TestOldLivenessKillLeavesTheCrashItsOwn(t *testing.T) {
	f := killFixture{state: "running", restarts: 4, exit: 1,
		finished: 20 * time.Second,
		message: "Liveness probe failed: dial tcp 10.0.0.9:8080: " +
			"connect: connection refused"}
	f.stop = "Container main failed liveness probe, will be restarted"
	f.stopAt = podNodeNow.Add(-20*time.Second - livenessNoteLead -
		time.Second)
	found := detectKill(t, f)

	require.NotEmpty(t, found)
	assert.Contains(t, found[0].Summary,
		"keeps exiting on its own with an error (exit code 1)")
}

// Before kills make a loop, the probe finding of such a container
// still says its liveness check fails, not that it exits on its own.
func TestLivenessStopBeforeALoopIsNotACrash(t *testing.T) {
	f := killFixture{state: "running", restarts: 1, exit: 1,
		finished: 20 * time.Second, message: livenessMessage,
		stop:   "Container main failed liveness probe, will be restarted",
		stopAt: podNodeNow.Add(-25 * time.Second)}
	found := detectKill(t, f)

	require.NotEmpty(t, found)
	for _, x := range found {
		assert.NotContains(t, x.Summary, "exiting on its own")
	}
	assert.Equal(t, reasons.LivenessProbeFailed, found[0].Reason)
}
