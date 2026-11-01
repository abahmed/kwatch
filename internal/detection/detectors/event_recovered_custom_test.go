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

// customModel is a resource with one Ready condition. namespace "" makes
// it cluster-scoped, as a ClusterPolicy is.
func customModel(
	namespace, status string, since time.Time,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := inventory.NewEntityID("kyverno.io", "clusterpolicy", namespace,
		"require-labels")
	key := kube.ConditionKey("Ready")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom:                inventory.Bool(true),
		key:                            inventory.Text(status),
		key + kube.AttrConditionSince:  inventory.Time(since),
		key + kube.AttrConditionReason: inventory.Text("Reconciled"),
	})
	return m, id
}

func noteUpdateFailed(
	m *inventory.Model, id inventory.EntityID, at time.Time, count int,
) {
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: id,
		Note: inventory.Note{
			At: at, FirstSeen: at, Source: "controller",
			Reason: "UpdateFailed", Message: "conflict", Count: count,
			Warning: true,
		},
	})
}

func eventFindings(
	m *inventory.Model, id inventory.EntityID, now time.Time,
) []detection.Finding {
	return detection.NewRegistry(nil, Event{}).Evaluate(m, now, id).Findings
}

func TestUnusualEventsIgnoresWarningsOlderThanCustomRecovery(t *testing.T) {
	for name, namespace := range map[string]string{
		"namespaced": "ops", "cluster-scoped": "",
	} {
		t.Run(name, func(t *testing.T) {
			ready := t0.Add(5 * time.Minute)
			m, id := customModel(namespace, "True", ready)
			noteUpdateFailed(m, id, t0, 3)

			assert.Empty(t, eventFindings(m, id, t0.Add(6*time.Minute)))
		})
	}
}

func TestUnusualEventsIgnoresNewWarningStreamOnHealthyCustomResource(
	t *testing.T,
) {
	for name, namespace := range map[string]string{
		"namespaced": "ops", "cluster-scoped": "",
	} {
		t.Run(name, func(t *testing.T) {
			m, id := customModel(namespace, "True", t0.Add(-time.Hour))
			noteUpdateFailed(m, id, t0, 12)

			assert.Empty(t, eventFindings(m, id, t0.Add(time.Minute)))
		})
	}
}

func TestUnusualEventsClearAsCustomResourceTurnsHealthy(t *testing.T) {
	m, id := customModel("ops", "False", t0)
	noteUpdateFailed(m, id, t0, 3)
	r := detection.NewRegistry(nil, Event{})
	require.Len(t, r.Evaluate(m, t0, id).Findings, 1)

	ready := kube.ConditionKey("Ready")
	later := t0.Add(time.Minute)
	put(m, id, later, map[string]inventory.Value{
		kube.AttrCustom:                 inventory.Bool(true),
		ready:                           inventory.Text("True"),
		ready + kube.AttrConditionSince: inventory.Time(later),
	})
	noteUpdateFailed(m, id, later.Add(time.Second), 3)

	assert.Empty(t, r.Evaluate(m, later.Add(time.Minute), id).Findings)
}

// noStatusCustom is a custom resource that reports no health at all.
func noStatusCustom(namespace string) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := inventory.NewEntityID("example.com", "order", namespace, "o")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom: inventory.Bool(true),
	})
	return m, id
}

func TestUnusualEventsSingleLaterEventIsNoRecurrenceOnCustomResource(
	t *testing.T,
) {
	for name, namespace := range map[string]string{
		"namespaced": "ops", "cluster-scoped": "",
	} {
		t.Run(name, func(t *testing.T) {
			m, id := noStatusCustom(namespace)
			noteUpdateFailed(m, id, t0, 1)
			later := t0.Add(EventWindow + time.Minute)
			noteUpdateFailed(m, id, later, 1)

			assert.Empty(t, eventFindings(m, id, later))
		})
	}
}

func TestUnusualEventsReportsThreeInWindowOnNoStatusCustomResource(
	t *testing.T,
) {
	for name, namespace := range map[string]string{
		"namespaced": "ops", "cluster-scoped": "",
	} {
		t.Run(name, func(t *testing.T) {
			m, id := noStatusCustom(namespace)
			noteUpdateFailed(m, id, t0, 3)

			found := eventFindings(m, id, t0.Add(time.Minute))
			require.Len(t, found, 1)
			assert.Equal(t, detection.Info, found[0].Severity)
		})
	}
}

func TestUnusualEventsCountsWarningsWhileCustomReadyIsNotTrue(t *testing.T) {
	for _, status := range []string{"Unknown", "False"} {
		t.Run(status, func(t *testing.T) {
			m, id := customModel("ops", status, t0.Add(5*time.Minute))
			noteUpdateFailed(m, id, t0, 3)

			assert.Len(t, eventFindings(m, id, t0.Add(6*time.Minute)), 1)
		})
	}
}

func TestUnusualEventsCountsWarningsWhileAnotherConditionFails(t *testing.T) {
	m, id := customModel("ops", "True", t0.Add(5*time.Minute))
	stalled := kube.ConditionKey("Stalled")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom:                   inventory.Bool(true),
		kube.ConditionKey("Ready"):        inventory.Text("True"),
		stalled:                           inventory.Text("True"),
		stalled + kube.AttrConditionSince: inventory.Time(t0),
	})
	noteUpdateFailed(m, id, t0, 3)

	assert.Len(t, eventFindings(m, id, t0.Add(6*time.Minute)), 1)
}

func TestReadySinceFallsBackToWhenCustomReadyWasSeen(t *testing.T) {
	m := newTestModel()
	id := newID("widget", "ops", "w")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrCustom:            inventory.Bool(true),
		kube.ConditionKey("Ready"): inventory.Text("True"),
	})

	since, ok := readySince(entityOf(m, id))
	assert.True(t, ok)
	assert.Equal(t, t0, since)
}

// Ready=Unknown past the reconcile window is a finding, and it ends the
// moment the controller says True, whatever the wait so far.
func TestCustomFindingClearsWhenReadyUnknownTurnsTrue(t *testing.T) {
	m, id := customModel("ops", "Unknown", t0.Add(-time.Hour))
	r := detection.NewRegistry(nil, Custom{})
	require.Len(t, r.Evaluate(m, t0, id).Findings, 1)

	key := kube.ConditionKey("Ready")
	put(m, id, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrCustom:               inventory.Bool(true),
		key:                           inventory.Text("True"),
		key + kube.AttrConditionSince: inventory.Time(t0.Add(time.Minute)),
	})
	assert.Empty(t, r.Evaluate(m, t0.Add(time.Minute), id).Findings)
}

// noteEvent records one Event object, identified by its origin.
func noteEvent(
	m *inventory.Model, id inventory.EntityID, origin string, at time.Time,
) {
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: id,
		Note: inventory.Note{
			At: at, Source: "operator", Reason: "SyncFailed", Message: "failed",
			Count: 1, Warning: true, Origin: origin,
		},
	})
}

func TestUnusualEventsReportsDistinctEventsOnNoStatusCustomResource(
	t *testing.T,
) {
	m, id := noStatusCustom("ops")
	for i, origin := range []string{"a", "b", "c"} {
		noteEvent(m, id, origin, t0.Add(time.Duration(i)*time.Minute))
	}

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(3*time.Minute)), entityOf(m, id))

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "SyncFailed 3 times")
}

func TestUnusualEventsCountsOnlyDistinctEventsInsideTheWindow(t *testing.T) {
	m, id := noStatusCustom("ops")
	noteEvent(m, id, "a", t0)
	noteEvent(m, id, "b", t0.Add(time.Hour))
	last := t0.Add(2 * time.Hour)
	noteEvent(m, id, "c", last)

	got := Event{}.Detect(
		testDetectorContext(m, last.Add(time.Minute)), entityOf(m, id))

	assert.Empty(t, got, "three events hours apart are no pattern")
}

func TestUnusualEventsAddsUpSightingsWithoutOrigin(t *testing.T) {
	m, id := noStatusCustom("ops")
	for i := range unusualEventMin {
		noteEntity(m, id, "SyncFailed", "failed", 1,
			t0.Add(time.Duration(i)*time.Minute))
	}

	got := Event{}.Detect(
		testDetectorContext(m, t0.Add(5*time.Minute)), entityOf(m, id))

	require.Len(t, got, 1)
}

func TestUnusualEventsStaySilentOnHealthyCustomResourceWithDistinctEvents(
	t *testing.T,
) {
	m, id := customModel("ops", "True", t0.Add(-time.Hour))
	for i, origin := range []string{"a", "b", "c"} {
		noteEvent(m, id, origin, t0.Add(time.Duration(i)*time.Minute))
	}

	assert.Empty(t, Event{}.Detect(
		testDetectorContext(m, t0.Add(3*time.Minute)), entityOf(m, id)))
}
