package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A controller that reports Ready=Unknown is still working, not failing:
// it gets the reconcile window (the one a controller that has not
// observed its spec gets) before the condition is reported.
func TestCustomReadyUnknownWaitsTheReconcileWindow(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "Unknown", "Why", "reconciling", t0)

	early := evaluate(Custom{}, m, t0.Add(DefaultCustomFailing), id, nil)
	late := evaluate(Custom{}, m, t0.Add(DefaultGenerationGrace), id, nil)

	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultGenerationGrace-DefaultCustomFailing,
		early.RecheckAfter)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, "Reports Ready=Unknown (Why)", late.Findings[0].Summary)
}

// Beside a condition that is a failure by itself, Unknown does not hold
// the report back: the shorter window applies and both are named.
func TestCustomReadyUnknownBesideAFailureUsesTheShortWindow(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "Unknown", "Why", "reconciling", t0)
	setCondition(m, id, "Failed", "True", "Boom", "it broke", t0)

	got := evaluate(Custom{}, m, t0.Add(DefaultCustomFailing), id,
		nil).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "Ready=Unknown")
	assert.Contains(t, got[0].Summary, "Failed=True")
}

// The generic rule keeps pace with the custom one, so a kind without a
// specialised detector does not report Unknown earlier.
func TestGenericReadyUnknownWaitsTheReconcileWindow(t *testing.T) {
	m, id := genericEntity(nil)
	setCondition(m, id, "Ready", "Unknown", "Why", "reconciling", t0)

	early := evaluate(Generic{}, m, t0.Add(DefaultConditionGrace), id, nil)
	late := evaluate(Generic{}, m, t0.Add(DefaultGenerationGrace), id, nil)

	assert.Empty(t, early.Findings)
	assert.Equal(t, DefaultGenerationGrace-DefaultConditionGrace,
		early.RecheckAfter)
	assert.Len(t, late.Findings, 1)
}
