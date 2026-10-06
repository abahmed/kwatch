package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A Warning event kwatch has no detector for becomes an informational
// finding once it repeats, with the event text quoted.
func TestEventUnknownWarningRepeatedIsReported(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n1")
	put(m, id, t0, nil)
	noteEntity(m, id, "SystemOOM", "System OOM encountered, victim "+
		"process: java", 3, t0.Add(time.Minute))

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(2*time.Minute)), entityOf(m, id))

	require.Len(t, got, 1)
	assert.Equal(t, "UnusualEvent.SystemOOM", got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, detection.ModeUnusualEvent, got[0].Mode)
	assert.Contains(t, got[0].Summary, "SystemOOM 3 times")
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "event", Value: "System OOM encountered, victim process: java"})
}

// One unknown event is noise, and an event the object's state already
// shows is never reported twice.
func TestEventUnknownWarningNeedsRepeatsAndSkipsKnownState(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPod, "shop", "api")
	put(m, id, t0, nil)
	noteEntity(m, id, "SomethingOdd", "once", 1, t0.Add(time.Minute))
	noteEntity(m, id, "BackOff", "Back-off restarting", 9,
		t0.Add(time.Minute))

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(2*time.Minute)), entityOf(m, id))

	assert.Empty(t, got)
}

// An event that comes back after its finding cleared is reported again:
// the count of the Event object keeps growing and its last time moves.
func TestEventUnusualRecurringEventIsReportedAgainAfterItCleared(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "shop", "web")
	put(m, id, t0, nil)
	detect := func(at time.Time) []detection.Finding {
		return Event{}.Detect(testDetectorContext(m, at), entityOf(m, id))
	}

	noteEntity(m, id, "FailedDeployModel", "Failed deploy model", 4, t0)
	require.Len(t, detect(t0.Add(time.Minute)), 1)
	assert.Empty(t, detect(t0.Add(EventWindow+time.Minute)),
		"the finding clears once the event stops")

	again := t0.Add(40 * time.Minute)
	noteEntity(m, id, "FailedDeployModel", "Failed deploy model", 5, again)
	got := detect(again.Add(time.Minute))
	require.Len(t, got, 1)
	assert.Equal(t, "UnusualEvent.FailedDeployModel", got[0].Reason)
	assert.Equal(t, again, got[0].Since)
}

// Each sighting may be a fresh Event with a count of one: the second
// sighting in a later quarter hour is still a recurrence, reported
// without inventing a count.
func TestEventUnusualFreshEventsThatKeepComingBackAreReported(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "shop", "web")
	put(m, id, t0, nil)
	noteEntity(m, id, "FailedDeployModel", "first", 1, t0)
	assert.Empty(t, Event{}.Detect(
		testDetectorContext(m, t0.Add(time.Minute)), entityOf(m, id)),
		"one sighting is noise")

	again := t0.Add(40 * time.Minute)
	noteEntity(m, id, "FailedDeployModel", "second", 1, again)
	got := Event{}.Detect(
		testDetectorContext(m, again.Add(time.Minute)), entityOf(m, id))
	require.Len(t, got, 1)
	assert.Equal(t, "Kubernetes reported FailedDeployModel again",
		got[0].Summary)
}

// The recheck lands just after the last event ages out of the window, so
// the finding clears then and not at the next event.
func TestEventUnusualRecheckFallsAfterTheWindowEdge(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindService, "shop", "web")
	put(m, id, t0, nil)
	noteEntity(m, id, "FailedDeployModel", "Failed deploy model", 4, t0)
	registry := detection.NewRegistry(nil, Event{})

	first := registry.Evaluate(m, t0.Add(time.Minute), id)
	require.Len(t, first.Findings, 1)
	require.Positive(t, first.RecheckAfter)

	again := registry.Evaluate(
		m, t0.Add(time.Minute).Add(first.RecheckAfter), id)
	assert.Empty(t, again.Findings)
}
