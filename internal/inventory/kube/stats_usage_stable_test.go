package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageStableRecordIsUnchangedForASteadyContainer(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := &memoryHistory{}
	h.observe(base, base, 500)
	h.seen = base
	first := h.stableRecord(base)

	for i := 1; i <= 6; i++ {
		at := base.Add(time.Duration(i) * usageSaveEvery)
		h.observe(base, at, 400)
		h.seen = at
		first = h.stableRecord(at)
	}
	// Two more saves: the peak and the hour did not change.
	at := base.Add(7 * usageSaveEvery)
	h.observe(base, at, 450)
	h.seen = at
	a := h.stableRecord(at)
	at = base.Add(8 * usageSaveEvery)
	h.observe(base, at, 420)
	h.seen = at
	b := h.stableRecord(at)

	assert.Equal(t, a, b, "nothing lasting changed: same stored record")
	assert.Equal(t, first, a)
}

func TestUsageStableRecordChangesOnANewPeakOrAfterAnHour(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := &memoryHistory{}
	h.observe(base, base, 500)
	h.seen = base
	before := h.stableRecord(base)

	at := base.Add(usageSaveEvery)
	h.observe(base, at, 900)
	h.seen = at
	peak := h.stableRecord(at)
	require.NotNil(t, peak.Run)
	assert.Equal(t, 900.0, peak.Run.Peak, "a new peak is stored")
	assert.NotEqual(t, before, peak)

	at = base.Add(usageRefreshEvery + usageSaveEvery)
	h.observe(base, at, 900)
	h.seen = at
	late := h.stableRecord(at)
	assert.Equal(t, 3, late.Run.Samples, "an hour on, the record is fresh")
	assert.Equal(t, at.Truncate(usageSaveEvery), late.Run.LastAt)
}

// A restart moves the current run into Prev: stored at once.
func TestUsageStableRecordChangesWhenTheContainerRestarts(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	h := &memoryHistory{}
	h.observe(base, base, 500)
	h.seen = base
	h.stableRecord(base)

	at := base.Add(usageSaveEvery)
	h.observe(at, at, 100)
	h.seen = at
	got := h.stableRecord(at)
	require.NotNil(t, got.Prev)
	assert.Equal(t, 500.0, got.Prev.Peak)
}
