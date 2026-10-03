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

// A pod that hit a FailedMount while starting and then became ready has
// recovered: the event is history. A failure after it became ready counts.
func TestEventStartFailureOvercomeByReadyPod(t *testing.T) {
	now := time.Date(2026, 10, 3, 16, 33, 0, 0, time.UTC)
	mount := now.Add(-52 * time.Second)
	for _, tc := range []struct {
		name       string
		ready      bool
		readySince time.Time
		want       int
	}{
		{"ready after the event", true, mount.Add(time.Second), 0},
		{"ready before the event", true, mount.Add(-time.Minute), 1},
		{"never ready", false, time.Time{}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel()
			id := inventory.CoreID(kube.KindPod, "ns", "tenant-0")
			model.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: "test", At: mount,
				Entity: id, Attributes: map[string]inventory.Value{
					kube.AttrReady:      inventory.Bool(tc.ready),
					kube.AttrReadySince: inventory.Time(tc.readySince),
				},
			})
			warn(model, id, mount, "FailedMount",
				"MountVolume.SetUp failed for volume \"kube-api-access\"")
			entity, _ := model.Entity(id)
			found := Event{}.Detect(testDetectorContext(model, now), entity)
			assert.Len(t, found, tc.want)
		})
	}
}

// A deleted Pod keeps its events for a while, but it no longer fails: its
// event findings must clear, or an incident stays open after the fix. An
// event about a kind kwatch does not watch still counts.
func TestEventFindingsClearWhenTheObjectIsDeleted(t *testing.T) {
	now := time.Date(2026, 10, 3, 16, 52, 0, 0, time.UTC)
	mount := now.Add(-90 * time.Second)
	pod := inventory.CoreID(kube.KindPod, "ns", "recovery-old")
	unwatched := inventory.EntityID{Kind: "widget", Namespace: "ns",
		Name: "w"}
	synced := func(kind inventory.Kind) bool { return kind == kube.KindPod }
	model := newTestModel()
	model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: mount, Entity: pod,
		Attributes: map[string]inventory.Value{}})
	warn(model, pod, mount, "FailedMount", "MountVolume.SetUp failed")
	warn(model, unwatched, mount, "FailedMount", "MountVolume.SetUp failed")
	registry := detection.NewRegistry(synced, Event{})

	assert.Len(t, registry.Evaluate(model, now, pod).Findings, 1,
		"a live pod with a recent failure still fails")
	model.Apply(inventory.Observation{Kind: inventory.Gone,
		Source: "test", At: now, Entity: pod})
	assert.Empty(t, registry.Evaluate(model, now, pod).Findings,
		"a deleted pod's events are history")
	assert.Len(t, registry.Evaluate(model, now, unwatched).Findings, 1,
		"events about unwatched kinds keep counting")
}
