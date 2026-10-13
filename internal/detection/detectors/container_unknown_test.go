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

func waitingContainer(reason, message string, since time.Time,
) inventory.Entity {
	return buildContainer(inventory.EntityID{Kind: kube.KindPod,
		Namespace: "default", Name: "pod-1"}, "app", since,
		map[string]inventory.Value{
			kube.AttrState:       inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(reason),
			kube.AttrMessage:     inventory.Text(message),
		})
}

// A waiting state kwatch has no name for is never assumed healthy: once
// it has lasted past the grace it is a failure that quotes the state.
func TestUnknownWaitingReasonIsAFailureOnceItLasts(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	c := waitingContainer("BrandNewKubeletState", "it said this",
		now.Add(-5*time.Minute))

	got := Container{}.Detect(testDetectorContext(nil, now), c)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.UnknownWaiting("BrandNewKubeletState"),
		got[0].Reason)
	assert.Equal(t, detection.ModeUnknownWaiting, got[0].Mode)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, detection.Failing, got[0].Health)
	assert.Contains(t, got[0].Summary, "BrandNewKubeletState")
	assert.Contains(t, got[0].Evidence,
		detection.Evidence{Label: "message", Value: "it said this"})
}

func TestUnknownWaitingReasonWaitsForItsGrace(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	c := waitingContainer("BrandNewKubeletState", "",
		now.Add(-30*time.Second))

	got := Container{}.Detect(testDetectorContext(nil, now), c)

	assert.Empty(t, got)
}

func TestKnownProgressAndEmptyWaitingReasonsStayQuiet(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	for _, reason := range []string{"ContainerCreating", "PodInitializing", ""} {
		c := waitingContainer(reason, "", now.Add(-time.Hour))
		assert.Empty(t, Container{}.Detect(
			testDetectorContext(nil, now), c), reason)
	}
}
