package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A finding that holds only inside a time window must ask to be looked at
// again when the window ends: nothing else touches the entity then, and a
// finding left standing after its evidence aged out is stale.

func TestBudgetOverlapRechecksWhenTheControllerFlagAgesOut(t *testing.T) {
	m := newTestModel()
	pod := labelledPod(m, "web-1", "app=web,tier=front")
	id := budget(m, "web", "app=web", 1)
	budget(m, "front", "tier=front", 1)
	warn(m, pod, t0, "MultiplePodDisruptionBudgets",
		"Pod belongs to multiple PodDisruptionBudgets")

	now := t0.Add(time.Minute)
	eval := evaluate(Budget{}, m, now, id, nil)
	later := evaluate(Budget{}, m, t0.Add(EventWindow+time.Nanosecond), id,
		nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, EventWindow-time.Minute+time.Nanosecond,
		eval.RecheckAfter)
	require.Len(t, later.Findings, 1)
	assert.True(t, later.Findings[0].Advisory,
		"the flag aged out: overlap alone is only advisory")
}

func TestPodPreemptedRechecksWhenThePreemptionLeavesTheWindow(t *testing.T) {
	model, victim := preemptFixture(t, 1, preemptMessage)

	eval := evaluate(NewPod(PodThresholds{}), model, podNodeNow, victim,
		nil)
	later := evaluate(NewPod(PodThresholds{}), model,
		podNodeNow.Add(preemptionWindow), victim, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, preemptionWindow-2*time.Minute+time.Nanosecond,
		eval.RecheckAfter)
	assert.Empty(t, later.Findings, "the preemption is history")
}

func TestPodPreemptedRepeatedlyRechecksWhenTheOldestLeavesTheWindow(
	t *testing.T,
) {
	model := newTestModel()
	id := preemptedPods(model, 4, time.Minute, 5*time.Minute,
		14*time.Minute)

	eval := evaluate(NewPod(PodThresholds{}), model, podNodeNow, id, nil)

	require.NotEmpty(t, eval.Findings)
	assert.Equal(t, preemptionWindow-14*time.Minute+time.Nanosecond,
		eval.RecheckAfter)
}
