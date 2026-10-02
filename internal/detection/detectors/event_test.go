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

func TestEventFailedMount(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	// Add a warning event
	model.Apply(inventory.Observation{
		Kind:   inventory.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: inventory.Note{
			Reason:  "FailedMount",
			Warning: true,
			Message: "Failed to mount volume",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, "FailedMount", findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "Volume")
}

func TestEventFailedCreate(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: "Deployment", Namespace: "default",
		Name: "app"}

	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	model.Apply(inventory.Observation{
		Kind:   inventory.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: inventory.Note{
			Reason:  "FailedCreate",
			Warning: true,
			Message: "Error creating pod",
			Count:   3,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, "FailedCreate", findings[0].Reason)
	assert.Contains(t, findings[0].Summary, "pods")
}

func TestEventIgnoredSuccess(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	// Normal event, not a failure
	model.Apply(inventory.Observation{
		Kind:   inventory.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: inventory.Note{
			Reason:  "Created",
			Warning: false,
			Message: "Pod created",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	assert.Empty(t, findings)
}

func TestEventStale(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-30 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	// Event outside the 15-minute window
	model.Apply(inventory.Observation{
		Kind:   inventory.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: inventory.Note{
			Reason:  "FailedMount",
			Warning: true,
			Message: "Failed to mount volume",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	assert.Empty(t, findings)
}

func TestEventReasonsAreUpstreamNames(t *testing.T) {
	notUpstream := []string{"FailedCallingWebhook", "FailedToScaleUp"}
	for _, reason := range notUpstream {
		_, listed := eventReasons[reason]
		assert.False(t, listed, reason)
	}
	_, listed := eventReasons["FailedCreate"]
	assert.True(t, listed)
}

// TestEventSourcesWithSameReasonFoldIntoOneFinding: notes are kept per
// source and reason, but findings are keyed by reason, so two sources
// reporting FailedMount must yield one finding with the newest message
// and the summed count.
func TestEventSourcesWithSameReasonFoldIntoOneFinding(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindPod, "ns", "app")
	put(m, id, t0, nil)
	note := func(source, message string, at time.Time, count int) {
		m.Apply(inventory.Observation{
			Kind: inventory.Noted, Source: source, At: at, Entity: id,
			Note: inventory.Note{
				At: at, Source: source, Reason: "FailedMount",
				Message: message, Count: count, Warning: true,
			},
		})
	}
	note("kubelet", "old", t0, 2)
	note("attachdetach", "new", t0.Add(time.Minute), 3)

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(2*time.Minute)), entityOf(m, id))

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Evidence,
		detection.Evidence{Label: "event", Value: "new"})
	assert.Contains(t, got[0].Evidence,
		detection.Evidence{Label: "occurrences", Value: "5"})
	assert.Equal(t, t0.Add(time.Minute), got[0].Since)
}
