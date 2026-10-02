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

// genericEntity observes an object of a kind no specialised detector
// owns.
func genericEntity(
	attrs map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID("widget", "default", "thing")
	put(m, id, t0, attrs)
	return m, id
}

func modes(findings []detection.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, string(f.Mode))
	}
	return out
}

func TestGenericConditionRules(t *testing.T) {
	cases := []struct {
		name, conditionType, status string
		want                        bool
	}{
		{"ready false", "Ready", "False", true},
		{"ready true", "Ready", "True", false},
		{"available false", "Available", "False", true},
		{"degraded true", "Degraded", "True", true},
		{"degraded false", "Degraded", "False", false},
		{"stalled true", "Stalled", "True", true},
		{"synced false", "Synced", "False", true},
		{"reconciled false", "Reconciled", "False", true},
		{"established false", "Established", "False", true},
		{"accepted false", "Accepted", "False", true},
		{"programmed false", "Programmed", "False", true},
		{"resolved refs false", "ResolvedRefs", "False", true},
		{"conflicted true", "Conflicted", "True", true},
		{"partially invalid true", "PartiallyInvalid", "True", true},
		{"partially invalid false", "PartiallyInvalid", "False", false},
		{"unknown status", "Ready", "Unknown", false},
		{"unlisted type", "Progressing", "False", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, id := genericEntity(nil)
			setCondition(m, id, tc.conditionType, tc.status, "Why",
				"what happened", t0)
			got := evaluate(Generic{}, m, t0.Add(time.Hour), id, nil)
			if !tc.want {
				assert.Empty(t, got.Findings)
				return
			}
			require.Len(t, got.Findings, 1)
			f := got.Findings[0]
			assert.Equal(t, "Condition."+tc.conditionType, string(f.Mode))
			assert.Equal(t, reasons.Condition(tc.conditionType), f.Reason)
			assert.True(t, t0.Equal(f.Since))
			assert.Contains(t, f.Evidence,
				detection.Evidence{Label: "reason", Value: "Why"})
		})
	}
}

func TestGenericConditionWaitsForGrace(t *testing.T) {
	m, id := genericEntity(nil)
	setCondition(m, id, "Ready", "False", "", "", t0)

	early := evaluate(Generic{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultConditionGrace-time.Minute, early.RecheckAfter)

	late := evaluate(Generic{}, m, t0.Add(DefaultConditionGrace), id, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, detection.Failing, late.Findings[0].Health)
}

func TestGenericGenerationLag(t *testing.T) {
	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrGeneration:  inventory.Number(4),
		kube.AttrObservedGen: inventory.Number(3),
	})
	early := evaluate(Generic{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultGenerationGrace-time.Minute, early.RecheckAfter)

	got := evaluate(Generic{}, m, t0.Add(DefaultGenerationGrace), id, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, "NotReconciling", string(got.Findings[0].Mode))
	assert.Equal(t, detection.Degraded, got.Findings[0].Health)
	assert.Contains(t, got.Findings[0].Evidence,
		detection.Evidence{Label: "observed generation", Value: "3"})
}

func TestGenericGenerationCaughtUpOrUnknown(t *testing.T) {
	later := t0.Add(time.Hour)
	caughtUp, id := genericEntity(map[string]inventory.Value{
		kube.AttrGeneration:  inventory.Number(4),
		kube.AttrObservedGen: inventory.Number(4),
	})
	assert.Empty(t, evaluate(Generic{}, caughtUp, later, id, nil).Findings)

	metadataOnly, id := genericEntity(map[string]inventory.Value{
		kube.AttrGeneration: inventory.Number(4),
		kube.AttrWatchMode:  inventory.Text(string(kube.WatchMetadata)),
	})
	assert.Empty(t,
		evaluate(Generic{}, metadataOnly, later, id, nil).Findings)
}

func TestGenericStuckDeletion(t *testing.T) {
	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
		kube.AttrFinalizers: inventory.Text(
			"example.com/cleanup,kubernetes"),
	})
	early := evaluate(Generic{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultDeletionGrace-time.Minute, early.RecheckAfter)

	got := evaluate(Generic{}, m, t0.Add(DefaultDeletionGrace), id, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, "StuckDeleting", string(got.Findings[0].Mode))
	assert.Equal(t,
		"Deletion is blocked by finalizers example.com/cleanup, kubernetes",
		got.Findings[0].Summary)
}

func TestGenericDeletionWithoutFinalizers(t *testing.T) {
	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrDeleting:      inventory.Bool(true),
		kube.AttrDeletingSince: inventory.Time(t0),
	})
	got := evaluate(Generic{}, m, t0.Add(time.Hour), id, nil)
	assert.Empty(t, got.Findings)
}

func TestGenericPhase(t *testing.T) {
	for _, phase := range []string{"Failed", "Error", "Lost"} {
		m, id := genericEntity(map[string]inventory.Value{
			kube.AttrPhase: inventory.Text(phase),
		})
		got := evaluate(Generic{}, m, t0, id, nil).Findings
		require.Len(t, got, 1, phase)
		assert.Equal(t, detection.Failing, got[0].Health, phase)
		assert.Equal(t, "Failed", string(got[0].Mode), phase)
	}

	m, id := genericEntity(map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending"),
	})
	early := evaluate(Generic{}, m, t0.Add(time.Minute), id, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultPendingGrace-time.Minute, early.RecheckAfter)
	got := evaluate(Generic{}, m, t0.Add(DefaultPendingGrace), id, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, detection.Degraded, got.Findings[0].Health)
	assert.Equal(t, "Pending", string(got.Findings[0].Mode))

	healthy, id := genericEntity(map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Bound"),
	})
	assert.Empty(t,
		evaluate(Generic{}, healthy, t0.Add(time.Hour), id, nil).Findings)
}

func TestGenericAbsentEntityIsQuiet(t *testing.T) {
	m := newTestModel()
	got := evaluate(Generic{}, m, t0, newID("widget", "a", "gone"), nil)
	assert.Empty(t, got.Findings)
}
