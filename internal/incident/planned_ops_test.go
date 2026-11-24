package incident

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var plannedBase = time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)

func plannedDown(day int, offset time.Duration) PlannedOp {
	at := plannedBase.AddDate(0, 0, day).Add(offset)
	return PlannedOp{Down: true, At: at, Last: at.Add(time.Minute),
		Scaled: 6}
}

// An operation is scheduled when the same kind began within the
// tolerance of its time of day on enough of the previous days.
func TestPlannedRecursOnEnoughEarlierDays(t *testing.T) {
	var h plannedHistory
	h = h.record(plannedDown(-3, 0), plannedBase)
	h = h.record(plannedDown(-1, 25*time.Minute), plannedBase)
	today := plannedDown(0, 0)

	seen, ok := h.scheduled(today)
	assert.Equal(t, 2, seen)
	assert.True(t, ok)

	up := today
	up.Down = false
	_, ok = h.scheduled(up)
	assert.False(t, ok, "a scale-up never matches a scale-down")

	early := plannedDown(0, -10*time.Minute)
	seen, ok = h.scheduled(early)
	assert.Equal(t, 1, seen, "35 minutes from the day before is "+
		"past the tolerance")
	assert.False(t, ok)
}

// The first sightings have no history: nothing is scheduled.
func TestPlannedFirstSightingIsNotScheduled(t *testing.T) {
	var h plannedHistory
	h = h.record(plannedDown(0, 0), plannedBase)
	_, ok := h.scheduled(h[0])
	assert.False(t, ok)
}

// Sightings of one operation merge; old ones are forgotten and the
// history stays bounded.
func TestPlannedHistoryMergesAndStaysBounded(t *testing.T) {
	var h plannedHistory
	first := plannedDown(0, 0)
	first.Workloads = []string{"deployment/shop/a"}
	h = h.record(first, plannedBase)
	later := plannedDown(0, 5*time.Minute)
	later.Workloads, later.Nodes = []string{"deployment/shop/b"}, 3
	h = h.record(later, plannedBase)
	require.Len(t, h, 1)
	assert.Equal(t, plannedBase, h[0].At)
	assert.Equal(t, 3, h[0].Nodes)
	assert.Equal(t, []string{"deployment/shop/a", "deployment/shop/b"},
		h[0].Workloads)

	now := plannedBase
	for i := range 3 * plannedMaxOps {
		at := plannedBase.Add(time.Duration(i) * time.Hour)
		now = at
		h = h.record(PlannedOp{Down: i%2 == 0, At: at}, now)
	}
	assert.Len(t, h, plannedMaxOps)
	h = h.prune(now.Add(plannedKeep + time.Hour))
	assert.Empty(t, h)
}

// A scale-down explains what it did until a grace after the scale-up
// that followed it, or for at most plannedMaxHold.
func TestPlannedEffectEndsAfterTheScaleUp(t *testing.T) {
	down := plannedDown(0, 0)
	h := plannedHistory{down}
	assert.Equal(t, down.At.Add(plannedMaxHold), h.effectEnd(down))

	upAt := plannedBase.Add(12*time.Hour + 45*time.Minute)
	h = append(h, PlannedOp{At: upAt, Last: upAt.Add(2 * time.Minute)})
	want := wakeEnd(h[1]).Add(PlannedGrace)
	assert.Equal(t, want, h.effectEnd(down))
}

// The digest line is for the newest scheduled operation that is over.
func TestLatestScheduledIsOverAndRecurs(t *testing.T) {
	var ops []PlannedOp
	for day := -2; day <= 0; day++ {
		ops = append(ops, plannedDown(day, 0))
	}
	today := ops[len(ops)-1]
	_, ok := LatestScheduled(ops, today.Last)
	assert.False(t, ok, "not over yet")
	got, ok := LatestScheduled(ops, today.Last.Add(time.Hour))
	require.True(t, ok)
	assert.Equal(t, today.At, got.At)
	assert.Equal(t, 2, got.Seen, fmt.Sprint(got))
}
