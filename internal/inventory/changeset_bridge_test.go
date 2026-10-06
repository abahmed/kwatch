package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A change that links two sets must not merge them into one longer than
// ChangeSetMaxSpan.
func TestChangeSetsDoNotBridgeBeyondTheMaxSpan(t *testing.T) {
	h := newHistoryModel(t)
	x := CoreID("deployment", "ops", "x")
	y := CoreID("deployment", "ops", "y")
	z := CoreID("deployment", "ops", "z")
	for _, id := range []EntityID{x, y, z} {
		h.apply(t, Observation{Kind: Observed, At: testTime, Entity: id})
	}
	edit := []FieldChange{{Path: "spec.replicas", After: "3"}}
	step := 90 * time.Second
	// Set A: app "a" every 90s for 12 minutes, ending with bob's edit.
	for i := range 8 {
		h.change(t, x, time.Duration(i)*step, Change{App: "a", Fields: edit})
	}
	h.change(t, x, 8*step, Change{Actor: "bob", App: "a", Fields: edit})
	// Set B starts 130s later, so it is not linked to A on its own.
	startB := 8*step + 130*time.Second
	h.change(t, y, startB, Change{Actor: "bob", App: "b", Fields: edit})
	for i := 1; i <= 8; i++ {
		h.change(t, y, startB+time.Duration(i)*step,
			Change{App: "b", Fields: edit})
	}
	require.Len(t, h.RecentChangeSets(time.Time{}), 2)

	// Bob's edit falls between them and is linked to both.
	h.change(t, z, 8*step+50*time.Second, Change{Actor: "bob", Fields: edit})

	sets := h.RecentChangeSets(time.Time{})
	total := 0
	for _, set := range sets {
		assert.LessOrEqual(t, set.End.Sub(set.Start), ChangeSetMaxSpan)
		total += len(set.Changes)
	}
	assert.Len(t, sets, 2, "the bridge joins one set and leaves the other")
	assert.Equal(t, 19, total)
}
