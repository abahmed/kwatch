package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// killedContainer is a container killed by its liveness probe at
// t0-2m, whose preStop hook failed as it died, and that is ready again
// from readyAt on (never, when readyAt is zero).
func killedContainer(
	m *inventory.Model, readyAt time.Time,
) inventory.EntityID {
	id := kube.ContainerID("shop", "api-1", "app")
	put(m, id, t0.Add(-3*time.Minute), map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	warn(m, id, t0.Add(-2*time.Minute), "FailedPreStopHook",
		"PreStopHook failed")
	if !readyAt.IsZero() {
		put(m, id, readyAt, map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(true)})
	}
	return id
}

// A failed preStop hook of a container that turned ready after it is
// history: it must not hold the incident for the event's whole window.
func TestContainerEventBeforeItTurnedReadyIsHistory(t *testing.T) {
	m := newTestModel()
	id := killedContainer(m, t0)

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	assert.Empty(t, eval.Findings)
}

// A restarted kwatch first sees the container ready after the event:
// the replayed event is history too.
func TestContainerEventReplayedAfterRestartIsHistory(t *testing.T) {
	m := newTestModel()
	id := kube.ContainerID("shop", "api-1", "app")
	warn(m, id, t0.Add(-9*time.Minute), "FailedPreStopHook",
		"PreStopHook failed")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	assert.Empty(t, eval.Findings)
}

// A container that is still not ready keeps its event finding.
func TestContainerEventWhileNotReadyStillCounts(t *testing.T) {
	m := newTestModel()
	id := killedContainer(m, time.Time{})

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "FailedPreStopHook", eval.Findings[0].Reason)
}

// An event after the container turned ready is news.
func TestContainerEventAfterItTurnedReadyStillCounts(t *testing.T) {
	m := newTestModel()
	id := kube.ContainerID("shop", "api-1", "app")
	put(m, id, t0.Add(-5*time.Minute), map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	warn(m, id, t0.Add(-time.Minute), "FailedPreStopHook",
		"PreStopHook failed")

	eval := evaluate(Event{}, m, t0.Add(time.Minute), id, nil)

	require.Len(t, eval.Findings, 1)
}
