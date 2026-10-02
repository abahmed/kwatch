package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// TestServiceSelectsNothingAfterEdit: a Service emptied by its own
// selector edit fails (it is not a symptom of its pods); an empty
// Service without such an edit stays quiet, and the edit stops counting
// after DefaultSelectorChangeWindow.
func TestServiceSelectsNothingAfterEdit(t *testing.T) {
	m, id := serviceWithSlice(0, 0, "", "80")
	quiet := evaluate(Service{}, m, t0.Add(2*time.Minute), id, nil)
	assert.Empty(t, quiet.Findings, "an empty Service is scaled to zero")

	_, err := m.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: t0, Entity: id, Change: inventory.Change{
			Fields: []inventory.FieldChange{{Path: "spec.selector",
				Before: "app=api", After: "app=apii"}}}})
	require.NoError(t, err)
	early := evaluate(Service{}, m, t0.Add(30*time.Second), id, nil)
	assert.Empty(t, early.Findings)

	got := evaluate(Service{}, m, t0.Add(2*time.Minute), id, nil).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
	assert.False(t, got[0].Symptom)
	assert.Contains(t, got[0].Summary, "spec.selector")

	late := evaluate(Service{}, m, t0.Add(2*time.Hour), id, nil)
	assert.Empty(t, late.Findings)
}
