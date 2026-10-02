package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

var timelineStart = time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)

func timelineAt(d time.Duration) time.Time { return timelineStart.Add(d) }

func kinds(entries []TimelineEntry) []TimelineKind {
	out := make([]TimelineKind, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Kind)
	}
	return out
}

func TestSortTimelineOrdersByAPITime(t *testing.T) {
	entries := []TimelineEntry{
		{At: timelineAt(time.Minute), Kind: TimelineChange},
		{At: timelineAt(0), Kind: TimelineDecision},
		{At: timelineAt(30 * time.Second), Kind: TimelineEvent},
	}
	SortTimeline(entries)
	assert.Equal(t, []TimelineKind{
		TimelineDecision, TimelineEvent, TimelineChange,
	}, kinds(entries))
}

func TestSortTimelineKeepsCausalOrderWithinSkew(t *testing.T) {
	entries := []TimelineEntry{
		{At: timelineAt(0), Kind: TimelineDecision},
		{At: timelineAt(time.Second), Kind: TimelineFinding},
		{At: timelineAt(1500 * time.Millisecond), Kind: TimelineChange},
		{At: timelineAt(10 * time.Second), Kind: TimelineEvent},
	}
	SortTimeline(entries)
	assert.Equal(t, []TimelineKind{
		TimelineChange, TimelineFinding, TimelineDecision, TimelineEvent,
	}, kinds(entries))
}

func TestSortTimelineUsesReceiveOrderForSameKind(t *testing.T) {
	entries := []TimelineEntry{
		{At: timelineAt(time.Second), Received: timelineAt(0),
			Kind: TimelineEvent, Reason: "first"},
		{At: timelineAt(0), Received: timelineAt(time.Second),
			Kind: TimelineEvent, Reason: "second"},
	}
	SortTimeline(entries)
	assert.Equal(t, "first", entries[0].Reason)
}

func TestTimelineTimeClampsSkew(t *testing.T) {
	received := timelineAt(0)
	assert.Equal(t, received, timelineTime(time.Time{}, received))
	assert.Equal(t, timelineAt(-time.Hour),
		timelineTime(timelineAt(-time.Hour), received))
	assert.Equal(t, timelineAt(TimelineSkew),
		timelineTime(timelineAt(TimelineSkew), received))
	assert.Equal(t, received, timelineTime(timelineAt(time.Minute), received))
}

func TestTimelineEntriesDescribeTheirSource(t *testing.T) {
	pod := inventory.CoreID("pod", "shop", "api-1")
	change := changeEntry(inventory.Change{
		Entity: pod, At: timelineAt(0), Observed: timelineAt(time.Second),
		Actor: "kubectl", Revision: "3",
		Fields: []inventory.FieldChange{{Path: "metadata.labels"}},
	})
	assert.Equal(t, "labels by kubectl, revision 3", change.Text)
	assert.Equal(t, "labels", change.Class)
	assert.Equal(t, timelineAt(time.Second), change.Received)

	finding := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Mode: "CrashLoop", Health: detection.Failing,
		Since: timelineAt(-time.Minute)}
	raised := findingEntry(detection.Transition{
		Kind: detection.Raised, Finding: finding}, timelineAt(0))
	assert.Equal(t, timelineAt(-time.Minute), raised.At)
	assert.Equal(t, "raised", raised.Action)
	assert.Equal(t, "failing", raised.Health)
	cleared := findingEntry(detection.Transition{
		Kind: detection.Cleared, Finding: finding}, timelineAt(0))
	assert.Equal(t, timelineAt(0), cleared.At)

	decision := decisionEntry(incident.Decision{Action: incident.Resolve,
		Reason: "healthy for 5m0s", Incident: incident.Incident{
			ID: "inc-1", Root: pod}}, timelineAt(0))
	assert.Equal(t, "resolve", decision.Action)
	assert.Equal(t, pod.String(), decision.Entity)
	assert.Equal(t, "inc-1", decision.Incident)
}

func TestHistoryStoreRoundTripsTimelineInOrder(t *testing.T) {
	store, ok := NewIncidentStore(openTempStore(t)).(HistoryStore)
	require.True(t, ok)
	entity := "pod/shop/api-1"
	require.NoError(t, store.AppendTimeline([]TimelineEntry{
		{Entity: entity, At: timelineAt(time.Second), Kind: TimelineDecision},
		{Entity: entity, At: timelineAt(1500 * time.Millisecond),
			Kind: TimelineChange},
		{Entity: "pod/shop/other", At: timelineAt(0), Kind: TimelineEvent},
	}))
	got, err := store.LoadTimeline(entity, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []TimelineKind{TimelineChange, TimelineDecision},
		kinds(got))
}
