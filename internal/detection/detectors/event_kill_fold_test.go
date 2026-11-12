package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// livenessFailed notes the kubelet's liveness Unhealthy event at at.
func livenessFailed(m *inventory.Model, id inventory.EntityID, at time.Time) {
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: id,
		Note: inventory.Note{
			At: at, Reason: "Unhealthy", Topic: "liveness", Count: 3,
			Message: "Liveness probe failed: connection refused",
			Warning: true,
		},
	})
}

// stoppingContainer is a not-ready container whose preStop hook failed
// at t0-2m.
func stoppingContainer(m *inventory.Model) inventory.EntityID {
	id := kube.ContainerID("shop", "api-1", "app")
	put(m, id, t0.Add(-5*time.Minute), map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	warn(m, id, t0.Add(-2*time.Minute), "FailedPreStopHook",
		"PreStopHook failed")
	return id
}

// The preStop hook that fails while the kubelet kills the container for
// its liveness probe is part of that kill, not a failure of its own.
func TestPreStopFailureOfALivenessKillIsPartOfTheKill(t *testing.T) {
	m := newTestModel()
	id := stoppingContainer(m)
	livenessFailed(m, id, t0.Add(-2*time.Minute-2*time.Second))

	eval := evaluate(Event{}, m, t0, id, nil)

	assert.Empty(t, eval.Findings)
}

// A liveness failure long before the stop event did not cause it.
func TestPreStopFailureLongAfterALivenessFailureCounts(t *testing.T) {
	m := newTestModel()
	id := stoppingContainer(m)
	livenessFailed(m, id, t0.Add(-10*time.Minute))

	eval := evaluate(Event{}, m, t0, id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "FailedPreStopHook", eval.Findings[0].Reason)
}
