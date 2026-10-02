package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func releaseOutcome(
	t *testing.T, h historyModel, now time.Duration,
) ChangeOutcome {
	t.Helper()
	outcomes := h.ChangeOutcomes(h.pod, time.Time{}, testTime.Add(now))
	require.NotEmpty(t, outcomes)
	return outcomes[0]
}

func TestChangeOutcomePendingThenHealthy(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))

	pending := releaseOutcome(t, h, time.Minute)
	assert.Equal(t, OutcomePending, pending.Outcome)
	assert.Equal(t, MinEffectWindow, pending.Window)

	healthy := releaseOutcome(t, h, 6*time.Minute)
	assert.Equal(t, OutcomeHealthy, healthy.Outcome)
}

func TestChangeOutcomeDegradedByDependent(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.config, 0, Change{
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	h.MarkHealth(h.pod, true, "CrashLoop", testTime.Add(2*time.Minute))

	outcome := releaseOutcome(t, h, 10*time.Minute)
	assert.Equal(t, OutcomeDegraded, outcome.Outcome)
	assert.Equal(t, []EntityID{h.pod}, outcome.Failed)
}

func TestChangeOutcomeIgnoresFailureAfterWindow(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.MarkHealth(h.pod, true, "CrashLoop", testTime.Add(20*time.Minute))
	assert.Equal(t, OutcomeHealthy, releaseOutcome(t, h, time.Hour).Outcome)
}

func TestChangeOutcomeRevertedAfterRecovery(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.MarkHealth(h.pod, true, "CrashLoop", testTime.Add(time.Minute))
	h.change(t, h.deployment, 4*time.Minute, imageChange("api:v2", "api:v1"))

	stillFailing := releaseOutcome(t, h, 5*time.Minute)
	assert.Equal(t, OutcomeDegraded, stillFailing.Outcome)

	h.MarkHealth(h.pod, false, "CrashLoop", testTime.Add(6*time.Minute))
	reverted := releaseOutcome(t, h, 7*time.Minute)
	assert.Equal(t, OutcomeReverted, reverted.Outcome)
	assert.Equal(t, testTime.Add(4*time.Minute), reverted.Reverted)
}

func TestChangeOutcomeRevertedByRevision(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.config, -time.Hour, Change{Revision: "hash-a",
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	h.change(t, h.config, 0, Change{Revision: "hash-b",
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	h.change(t, h.config, time.Minute, Change{Revision: "hash-a",
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	outcomes := h.ChangeOutcomes(h.config, testTime.Add(-time.Minute),
		testTime.Add(10*time.Minute))
	require.Len(t, outcomes, 1)
	assert.Equal(t, OutcomeReverted, outcomes[0].Outcome)
}

func TestEffectWindowFollowsReadyBaseline(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	set := h.ChangeSets(h.deployment, time.Time{})[0]
	for i := 0; i < 5; i++ {
		h.Baselines().Add(h.deployment, MetricReadySeconds, 600, testTime)
	}
	assert.Equal(t, 20*time.Minute, h.EffectWindow(set))
	for i := 0; i < 20; i++ {
		h.Baselines().Add(h.deployment, MetricReadySeconds, 3600, testTime)
	}
	assert.Equal(t, MaxEffectWindow, h.EffectWindow(set))
}
