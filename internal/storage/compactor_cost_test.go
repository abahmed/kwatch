package storage

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appendMany adds n change entries one minute apart, in one transaction.
func appendMany(t *testing.T, s *Store, n int) {
	t.Helper()
	entries := make([]Entry[string], n)
	for i := range entries {
		entries[i] = Entry[string]{
			Entity: fmt.Sprintf("pod-%04d", i), At: minute(i), Value: "c",
		}
	}
	require.NoError(t, ChangeLog[string](s).AppendAll(entries))
}

// A pass over data that has nothing to expire or evict must not take
// the write lock at all.
func TestCompactorPassWithNothingToDeleteOpensNoWriteTransaction(
	t *testing.T,
) {
	clock := newFakeClock()
	clock.Set(minute(3000))
	s := openClaimed(t, clock)
	appendMany(t, s, 2000)
	policy := DefaultPolicy()
	policy.Batch = 10
	before := s.counters.writeTxs.Load()

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Zero(t, result.Expired+result.Evicted)
	assert.Equal(t, before, s.counters.writeTxs.Load())
}

// Retention reads many keys per read transaction and writes one batch
// per transaction, so expiring n entries costs about n/batch writes.
func TestCompactorRetentionCostIsBoundedByBatches(t *testing.T) {
	clock := newFakeClock()
	clock.Set(minute(1000).Add(DefaultLogRetention))
	s := openClaimed(t, clock)
	appendMany(t, s, 1500)
	policy := DefaultPolicy()
	policy.Batch = 100
	before := s.counters.writeTxs.Load()

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1000, result.Expired)
	assert.Equal(t, uint64(10), s.counters.writeTxs.Load()-before)
}

// The size cap finds every victim it needs in one scan and deletes them
// in batch-sized transactions, instead of rescanning per batch.
func TestCompactorEvictionScansOnce(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	appendMany(t, s, 1000)
	size := entrySize(t, "pod-0000", "c")
	policy := Policy{SizeCap: logicalTotal(t, s) - 150*size, Batch: 10}
	scans, writes := s.counters.scans.Load(), s.counters.writeTxs.Load()

	evicted, err := NewCompactor(s, policy).enforceCaps(
		context.Background(), 0)

	require.NoError(t, err)
	assert.Equal(t, 150, evicted)
	assert.Equal(t, uint64(2), s.counters.scans.Load()-scans,
		"one size measurement and one victim scan")
	assert.Equal(t, uint64(15), s.counters.writeTxs.Load()-writes)
	got := collect(t, ChangeLog[string](s), "pod-0149", minute(0), minute(1000))
	assert.Empty(t, got, "the oldest entries went first")
	assert.Len(t, collect(t, ChangeLog[string](s), "pod-0150",
		minute(0), minute(1000)), 1)
}

// A canceled context stops a scan midway instead of after the bucket.
func TestCompactorScanStopsOnCancel(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	appendMany(t, s, 3000)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	expired := func(_, _ []byte) bool {
		if calls++; calls == 10 {
			cancel()
		}
		return false
	}

	_, _, err := s.scanExpired(ctx, Changes, nil, 100, expired)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, calls, 3000)
}
