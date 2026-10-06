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

type killFixture struct {
	state    string
	waiting  string
	restarts float64
	exit     float64
	finished time.Duration
	message  string
	// noteAge is how long ago the event was last seen; default 1m.
	noteAge time.Duration
}

func detectKill(t *testing.T, f killFixture) []detection.Finding {
	t.Helper()
	model := newTestModel()
	id := kube.ContainerID("default", "app", "main")
	attrs := map[string]inventory.Value{
		kube.AttrState:        inventory.Text(f.state),
		kube.AttrRestarts:     inventory.Number(f.restarts),
		kube.AttrLastExitCode: inventory.Number(f.exit),
		kube.AttrLastFinished: inventory.Time(podNodeNow.Add(-f.finished)),
		kube.AttrReady:        inventory.Bool(true),
		kube.AttrStartedAt:    inventory.Time(podNodeNow.Add(-time.Hour)),
	}
	if f.waiting != "" {
		attrs[kube.AttrStateReason] = inventory.Text(f.waiting)
	}
	observeEntity(model, id, podNodeNow.Add(-10*time.Minute), attrs)
	if f.message != "" {
		if f.noteAge == 0 {
			f.noteAge = time.Minute
		}
		noteEntity(model, id, "Unhealthy", f.message, 3,
			podNodeNow.Add(-f.noteAge))
	}
	return classified(Container{}.Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
}

const livenessMessage = "Liveness probe failed: HTTP probe failed with " +
	"statuscode: 500"

func TestLivenessKillsReadAsACrashLoop(t *testing.T) {
	cases := map[string]killFixture{
		"running between kills": {state: "running", restarts: 4,
			exit: 143, finished: 20 * time.Second,
			message: livenessMessage},
		"in back-off": {state: "waiting", waiting: "CrashLoopBackOff",
			restarts: 4, exit: 143, finished: 20 * time.Second,
			message: livenessMessage},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			found := detectKill(t, f)
			require.Len(t, found, 1)
			assert.Equal(t, reasons.LivenessKilled, found[0].Reason)
			assert.True(t, found[0].Mode.Within(detection.ModeCrashLoop))
			assert.Contains(t, found[0].Summary,
				"keeps being killed by its liveness probe")
		})
	}
}

func TestLivenessKillNeedsALoopOfKills(t *testing.T) {
	cases := map[string]killFixture{
		"one kill": {state: "running", restarts: 1, exit: 143,
			finished: 20 * time.Second, message: livenessMessage},
		"not SIGTERM": {state: "running", restarts: 4, exit: 1,
			finished: 20 * time.Second, message: livenessMessage},
		"readiness only": {state: "running", restarts: 4, exit: 143,
			finished: 20 * time.Second,
			message:  "Readiness probe failed: refused"},
		"probe event older than the last kill": {state: "running",
			restarts: 4, exit: 143, finished: 20 * time.Second,
			message: livenessMessage, noteAge: 4 * time.Minute},
		"long ago": {state: "running", restarts: 4, exit: 143,
			finished: time.Hour, message: livenessMessage},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			for _, got := range detectKill(t, f) {
				assert.NotEqual(t, reasons.LivenessKilled, got.Reason)
			}
		})
	}
}

// TestLivenessProbeKillOnceIsQuiet: three failed probes that ended in
// one restart, with the container ready again, are not a finding.
func TestLivenessProbeKillOnceIsQuiet(t *testing.T) {
	found := detectKill(t, killFixture{state: "running", restarts: 1,
		exit: 143, finished: 20 * time.Second, message: livenessMessage})
	assert.Empty(t, found)
}
