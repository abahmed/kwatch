package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pinned data (here, state values) over the cap cannot be evicted. The
// pass keeps a quarter of the cap as history and reports the store as
// over its cap, instead of deleting all history on every pass.
func TestCompactorPinnedDataOverCapKeepsSomeHistory(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	appendMany(t, s, 400)
	history := logicalTotal(t, s)
	big := strings.Repeat("p", 4096)
	for _, key := range []string{"a", "b", "c", "d", "e", "f"} {
		require.NoError(t, state(s).Put(key, big))
	}
	policy := Policy{SizeCap: history, Batch: 50}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.True(t, result.OverCap)
	assert.True(t, s.Stats().OverCap)
	assert.Greater(t, result.PinnedBytes, policy.SizeCap)
	left := result.Bytes - result.PinnedBytes
	assert.LessOrEqual(t, left, policy.SizeCap/minHistoryDivisor)
	assert.Greater(t, left, policy.SizeCap/minHistoryDivisor-
		entrySize(t, "pod-0000", "c"), "history is kept, not wiped")

	again, err := NewCompactor(s, policy).Pass(context.Background())
	require.NoError(t, err)
	assert.Zero(t, again.Evicted, "the next pass deletes nothing more")
	assert.True(t, again.OverCap)
}

// History may use the room pinned data leaves under the cap.
func TestCompactorCountsOnlyEvictableBytesAgainstRoomLeft(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	appendMany(t, s, 100)
	require.NoError(t, state(s).Put("pinned", strings.Repeat("p", 512)))
	size := entrySize(t, "pod-0000", "c")
	policy := Policy{SizeCap: logicalTotal(t, s) - 10*size, Batch: 50}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 10, result.Evicted)
	assert.False(t, result.OverCap)
	assert.False(t, s.Stats().OverCap)
	assert.LessOrEqual(t, result.Bytes, policy.SizeCap)
}

// The timeline is bounded by its own cap, oldest entries first, so a
// busy cluster cannot fill the file with it.
func TestCompactorTimelineCapEvictsOldestTimeline(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	entries := make([]Entry[string], 1000)
	for i := range entries {
		entries[i] = Entry[string]{
			Entity: fmt.Sprintf("pod-%04d", i), At: minute(i), Value: "t",
		}
	}
	require.NoError(t, TimelineLog[string](s).AppendAll(entries))
	size := entrySize(t, "pod-0000", "t")
	policy := Policy{TimelineCap: 400 * size, Batch: 10}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 600, result.Evicted)
	sizes, err := s.LogicalSize()
	require.NoError(t, err)
	assert.Equal(t, 400*size, sizes[Timeline])
	assert.Empty(t, collect(t, TimelineLog[string](s), "pod-0599",
		time.Time{}, time.Time{}), "the oldest went first")
	assert.Len(t, collect(t, TimelineLog[string](s), "pod-0600",
		time.Time{}, time.Time{}), 1)
}
