package storage

import (
	"context"
	"strings"
	"testing"

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
