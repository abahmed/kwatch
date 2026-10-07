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

// killedWhileStarting is a container that liveness gives 40s (10s delay
// + 3 x 10s) and kills again and again after about 40s runs.
func killedWhileStarting(probes string) map[string]inventory.Value {
	return map[string]inventory.Value{
		kube.AttrState:            inventory.Text("running"),
		kube.AttrRestarts:         inventory.Number(5),
		kube.AttrLastExitCode:     inventory.Number(143),
		kube.AttrLastStarted:      inventory.Time(t0.Add(-70 * time.Second)),
		kube.AttrLastFinished:     inventory.Time(t0.Add(-30 * time.Second)),
		kube.AttrLivenessDelay:    inventory.Number(10),
		kube.AttrLivenessPeriod:   inventory.Number(10),
		kube.AttrLivenessFailures: inventory.Number(3),
		kube.AttrProbes:           inventory.Text(probes),
	}
}

func livenessStartRig(
	attrs map[string]inventory.Value, starts ...float64,
) *baselineRig {
	r := newBaselineRig(attrs)
	for i, seconds := range starts {
		r.m.Baselines().Add(r.dep, inventory.MetricStartSeconds, seconds,
			t0.Add(-time.Duration(i+1)*time.Hour))
	}
	noteEntity(r.m, r.container, "Unhealthy", livenessMessage, 3,
		t0.Add(-time.Minute))
	return r
}

func liveFinding(t *testing.T, r *baselineRig) detection.Finding {
	t.Helper()
	for _, f := range r.findings() {
		if f.Reason == reasons.LivenessKilled {
			return f
		}
	}
	require.Fail(t, "no liveness finding", "%v", r.findings())
	return detection.Finding{}
}

// TestLivenessKillShowsWhatTheStartNeeds: the workload's last starts
// took about 75s, liveness gives 40s, and each run ended before 75s.
func TestLivenessKillShowsWhatTheStartNeeds(t *testing.T) {
	r := livenessStartRig(killedWhileStarting("readiness,liveness"),
		74, 75, 76, 75, 90)
	found := liveFinding(t, r)
	assert.Equal(t, "40s (10s delay + 3 × 10s)",
		evidenceOf(found, detection.EvidenceLivenessGives))
	assert.Equal(t, "75s", evidenceOf(found, detection.EvidenceUsualStart))
	assert.Equal(t, "5", evidenceOf(found, detection.EvidenceStartSamples))
	assert.Empty(t, evidenceOf(found, detection.EvidenceKilledBeforeReady))
}

// TestLivenessKillWithoutHistoryStatesOnlyTheFact: nothing says how
// long a start takes, so only "killed before ready" is written, and
// only for a container that has a readiness probe.
func TestLivenessKillWithoutHistoryStatesOnlyTheFact(t *testing.T) {
	r := livenessStartRig(killedWhileStarting("readiness,liveness"))
	found := liveFinding(t, r)
	assert.Equal(t, "true",
		evidenceOf(found, detection.EvidenceKilledBeforeReady))
	assert.Empty(t, evidenceOf(found, detection.EvidenceUsualStart))
	assert.Empty(t, evidenceOf(found, detection.EvidenceStartSamples))

	r = livenessStartRig(killedWhileStarting("liveness"))
	found = liveFinding(t, r)
	assert.Empty(t, evidenceOf(found, detection.EvidenceLivenessGives),
		"without a readiness probe the container is ready when it runs")
}

// TestLivenessKillOfAStartedContainerIsNotAStartProblem: the run
// outlasted the usual start, so liveness did not cut the start short.
func TestLivenessKillOfAStartedContainerIsNotAStartProblem(t *testing.T) {
	attrs := killedWhileStarting("readiness,liveness")
	attrs[kube.AttrLastStarted] = inventory.Time(t0.Add(-300 * time.Second))
	r := livenessStartRig(attrs, 74, 75, 76)
	found := liveFinding(t, r)
	assert.Empty(t, evidenceOf(found, detection.EvidenceLivenessGives))
}

// TestLivenessThatAllowsTheStartAddsNothing: liveness gives longer than
// the starts take.
func TestLivenessThatAllowsTheStartAddsNothing(t *testing.T) {
	r := livenessStartRig(killedWhileStarting("readiness,liveness"),
		20, 22, 25)
	found := liveFinding(t, r)
	assert.Empty(t, evidenceOf(found, detection.EvidenceLivenessGives))
}

// TestLivenessKillNotesTheSameCheck: a liveness probe that runs the
// readiness check is written down as evidence.
func TestLivenessKillNotesTheSameCheck(t *testing.T) {
	attrs := killedWhileStarting("readiness,liveness")
	attrs[kube.AttrLivenessSameCheck] = inventory.Bool(true)
	found := liveFinding(t, livenessStartRig(attrs))
	assert.Equal(t, "true",
		evidenceOf(found, detection.EvidenceLivenessSameCheck))
}
