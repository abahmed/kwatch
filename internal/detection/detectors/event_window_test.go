package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The recheck lands just after the last known-reason event leaves the
// window, so the finding clears then and not at the next event.
func TestEventRecheckFallsAfterTheWindowEdge(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPod, "shop", "web-1")
	put(m, id, t0.Add(-time.Hour), map[string]inventory.Value{})
	warn(m, id, t0, "FailedMount", "mount failed")
	registry := detection.NewRegistry(nil, Event{})

	first := registry.Evaluate(m, t0.Add(time.Minute), id)
	require.Len(t, first.Findings, 1)
	require.Positive(t, first.RecheckAfter)

	again := registry.Evaluate(m, t0.Add(time.Minute).Add(first.RecheckAfter),
		id)
	assert.Empty(t, again.Findings)
}

// The load balancer finding must clear on the recheck, not stay one
// nanosecond past it.
func TestLoadBalancerRecheckFallsAfterTheWindowEdge(t *testing.T) {
	m := newTestModel()
	svc := newID(kube.KindService, "ns", "web")
	put(m, svc, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})
	warn(m, svc, t0, "SyncLoadBalancerFailed", "quota exceeded")

	first := evaluate(Service{}, m, t0.Add(time.Minute), svc, nil)
	require.Len(t, syncFailures(first), 1)
	require.Positive(t, first.RecheckAfter)

	again := evaluate(Service{}, m,
		t0.Add(time.Minute).Add(first.RecheckAfter), svc, nil)
	assert.Empty(t, syncFailures(again))
}

// The same holds for the CronJob controller's missed-run events.
func TestMissedRunsRecheckFallsAfterTheWindowEdge(t *testing.T) {
	m := newTestModel()
	cron := newID(kube.KindCronJob, "ns", "nightly")
	put(m, cron, t0.Add(-time.Hour), map[string]inventory.Value{})
	warn(m, cron, t0, "MissSchedule", "Missed scheduled time to start")

	first := evaluate(Schedule{}, m, t0.Add(time.Minute), cron, nil)
	require.Len(t, first.Findings, 1)
	require.Positive(t, first.RecheckAfter)

	again := evaluate(Schedule{}, m,
		t0.Add(time.Minute).Add(first.RecheckAfter), cron, nil)
	assert.Empty(t, again.Findings)
}

func syncFailures(eval detection.Evaluation) []detection.Finding {
	var out []detection.Finding
	for _, f := range eval.Findings {
		if f.Reason == "LoadBalancerSyncFailed" {
			out = append(out, f)
		}
	}
	return out
}
