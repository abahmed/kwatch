package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestEventFailedMount(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	// Add a warning event
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: knowledge.Note{
			Reason:  "FailedMount",
			Warning: true,
			Message: "Failed to mount volume",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, "FailedMount", signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "Volume")
}

func TestEventFailedCreate(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: "Deployment", Namespace: "default",
		Name: "app"}

	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	model.Apply(knowledge.Fact{
		Kind:   knowledge.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: knowledge.Note{
			Reason:  "FailedCreate",
			Warning: true,
			Message: "Error creating pod",
			Count:   3,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, "FailedCreate", signals[0].Reason)
	assert.Contains(t, signals[0].Summary, "pods")
}

func TestEventIgnoredSuccess(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	// Normal event, not a failure
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: knowledge.Note{
			Reason:  "Created",
			Warning: false,
			Message: "Pod created",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	assert.Empty(t, signals)
}

func TestEventStale(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-30 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: "Pod", Namespace: "default", Name: "app"}

	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	// Event outside the 15-minute window
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Noted,
		Source: "test",
		At:     since,
		Entity: id,
		Note: knowledge.Note{
			Reason:  "FailedMount",
			Warning: true,
			Message: "Failed to mount volume",
			Count:   1,
		},
	})

	entity, _ := model.Entity(id)
	detector := Event{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	assert.Empty(t, signals)
}
