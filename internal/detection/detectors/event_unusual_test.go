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
