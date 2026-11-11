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

// staleCheck evaluates id with d, first over a stale watch and then over
// a fresh one, and returns both evaluations.
func staleCheck(
	t *testing.T, d detection.Detector, m *inventory.Model,
	id inventory.EntityID, at time.Time,
) (stale, fresh detection.Evaluation) {
	t.Helper()
	isFresh := false
	r := detection.NewRegistry(nil, d)
	r.SetFresh(func(kind inventory.Kind) bool {
		assert.Equal(t, id.Kind, kind)
		return isFresh
	})
	stale = r.Evaluate(m, at, id)
	isFresh = true
	return stale, r.Evaluate(m, at, id)
}

// A custom resource whose watch keeps failing holds the conditions it had
// when the watch broke: a failure that looks sustained raises nothing,
// and an earlier finding is kept, not cleared.
func TestCustomIsUndecidedWhileItsWatchIsStale(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "False", "Broken", "boom", t0)

	stale, fresh := staleCheck(t, Custom{}, m, id, t0.Add(time.Hour))

	assert.Empty(t, stale.Findings)
	assert.Equal(t, []string{"custom-resource"}, stale.UnsyncedBy)
	assert.NotZero(t, stale.RecheckAfter, "freshness is checked again")
	previous := []detection.Finding{{Entity: id,
		Reason: reasons.CustomResourceFailure, Detector: "custom-resource"}}
	assert.Len(t, stale.Keeping(previous), 1, "held, not cleared")
	require.Len(t, fresh.Findings, 1, "a fresh watch raises")
}

// A recovered-looking custom resource on a stale watch does not clear
// what was raised: the recovery may be what the watch never saw undone.
func TestCustomHoldsAFindingOverAStaleRecovery(t *testing.T) {
	m, id := customResource("widget", true)
	setCondition(m, id, "Ready", "True", "", "", t0)

	stale, fresh := staleCheck(t, Custom{}, m, id, t0.Add(time.Hour))

	previous := []detection.Finding{{Entity: id,
		Reason: reasons.CustomResourceFailure, Detector: "custom-resource"}}
	assert.Len(t, stale.Keeping(previous), 1)
	assert.Empty(t, fresh.Keeping(previous), "a fresh recovery clears")
}

func TestGenericConditionsAreUndecidedWhileTheWatchIsStale(t *testing.T) {
	m, id := genericEntity(nil)
	setCondition(m, id, "Ready", "False", "Broken", "", t0)

	stale, fresh := staleCheck(t, Generic{}, m, id, t0.Add(time.Hour))

	assert.Empty(t, stale.Findings)
	assert.Equal(t, []string{"generic-health"}, stale.UnsyncedBy)
	require.Len(t, fresh.Findings, 1, "a fresh watch raises")
	assert.Len(t, stale.Keeping(fresh.Findings), 1, "held, not cleared")
}

// The kind-specific checks hold too: an approved CSR that a frozen cache
// still shows without a certificate raises nothing while stale.
func TestCustomKindChecksHoldWhileTheWatchIsStale(t *testing.T) {
	m := newTestModel()
	id := csr(m, "example.com/signer", false)
	setCondition(m, id, "Approved", "True", "AutoApproved", "", t0)

	stale, fresh := staleCheck(t, Custom{}, m, id, t0.Add(6*time.Minute))

	assert.Empty(t, stale.Findings, "a stale kind does not raise")
	assert.Equal(t, []string{"custom-resource"}, stale.UnsyncedBy)
	assert.NotZero(t, stale.RecheckAfter, "freshness is checked again")
	require.Len(t, fresh.Findings, 1, "a fresh watch raises")
	assert.Len(t, stale.Keeping(fresh.Findings), 1, "held, not cleared")
}
