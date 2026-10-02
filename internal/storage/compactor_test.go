package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

const day = 24 * time.Hour

// entrySize is the logical size of one log entry with value v.
func entrySize(t *testing.T, entity, v string) int64 {
	t.Helper()
	data, err := encode(v, time.Time{}, time.Time{})
	require.NoError(t, err)
	return int64(len(logKey(entity, time.Time{}, 0)) + len(data))
}

func logicalTotal(t *testing.T, s *Store) int64 {
	t.Helper()
	sizes, err := s.LogicalSize()
	require.NoError(t, err)
	return total(sizes)
}

func TestCompactorRetentionDeletesOnlyExpired(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	now := clock.Now()
	changes := ChangeLog[string](s)
	for _, age := range []time.Duration{40 * day, 31 * day, 29 * day, 0} {
		require.NoError(t, changes.Append("pod", now.Add(-age), "c"))
	}
	require.NoError(t, AuditLog[string](s).Append(
		"decisions", now.Add(-31*day), "a"))
	incidents := IncidentRecords[string](s)
	require.NoError(t, incidents.PutUntil("gone", "r", now.Add(-time.Hour)))
	require.NoError(t, incidents.PutUntil("kept", "r", now.Add(time.Hour)))
	require.NoError(t, incidents.Put("open", "o"))
	require.NoError(t, BaselineValues[string](s).Put("wl", "b"))
	policy := DefaultPolicy()
	policy.Batch = 2

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, result.Expired)
	assert.Len(t, collect(t, changes, "pod", time.Time{}, time.Time{}), 2)
	assert.Empty(t, collect(t, AuditLog[string](s), "decisions",
		time.Time{}, time.Time{}))
	var left []string
	require.NoError(t, s.view(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(Incidents)).ForEach(func(k, _ []byte) error {
			left = append(left, string(k))
			return nil
		})
	}))
	assert.Equal(t, []string{"kept", "open"}, left)
	_, found, err := BaselineValues[string](s).Get("wl")
	require.NoError(t, err)
	assert.True(t, found, "baselines never expire")
	assert.Equal(t, uint64(4), s.Stats().Expired)
}

func TestCompactorSizeCapDeletesOldestFirstInBatches(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	value := strings.Repeat("x", 100)
	put := func(l Log[string], entity string, at int) {
		require.NoError(t, l.Append(entity, minute(at), value))
	}
	put(AuditLog[string](s), "str", 0)
	put(ChangeLog[string](s), "pod", 1)
	put(TimelineLog[string](s), "inc", 2)
	for i := 3; i < 8; i++ {
		put(EvidenceLog[string](s), "inc", i)
	}
	require.NoError(t, IncidentRecords[string](s).Put("open", value))
	require.NoError(t, StateValues[string](s).Put("k", value))
	size := entrySize(t, "pod", value)
	policy := Policy{SizeCap: logicalTotal(t, s) - 4*size + 1, Batch: 2}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, result.Evicted)
	assert.LessOrEqual(t, result.Bytes, policy.SizeCap)
	for _, l := range []Log[string]{
		AuditLog[string](s), ChangeLog[string](s), TimelineLog[string](s),
	} {
		assert.Empty(t, collect(t, l, "str", time.Time{}, time.Time{}))
	}
	assert.Empty(t, collect(t, ChangeLog[string](s), "pod",
		time.Time{}, time.Time{}))
	assert.Empty(t, collect(t, TimelineLog[string](s), "inc",
		time.Time{}, time.Time{}))
	assert.Len(t, collect(t, EvidenceLog[string](s), "inc",
		time.Time{}, time.Time{}), 4, "only the oldest evidence entry goes")
	_, found, _ := IncidentRecords[string](s).Get("open")
	assert.True(t, found, "open incidents are never evicted")
	_, found, _ = state(s).Get("k")
	assert.True(t, found, "state is never evicted")
}

// The incidents bucket mirrors the incident manager. Evicting resolved
// records would only make the next save write them back, so the size
// cap leaves them to their expiry.
func TestCompactorSizeCapNeverEvictsIncidents(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	incidents := IncidentRecords[string](s)
	require.NoError(t, incidents.PutUntil("resolved", "r",
		clock.Now().Add(day)))
	require.NoError(t, incidents.Put("open", "o"))
	policy := Policy{SizeCap: 1}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, result.Evicted)
	assert.Equal(t, result.Bytes, result.PinnedBytes)
	for _, key := range []string{"resolved", "open"} {
		_, found, _ := incidents.Get(key)
		assert.True(t, found, key)
	}

	clock.Advance(2 * day)
	result, err = NewCompactor(s, policy).Pass(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Expired, "expiry still removes it")
}

func TestCompactorEvidenceCapTouchesOnlyEvidence(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	value := strings.Repeat("e", 50)
	evidence := EvidenceLog[string](s)
	for i := 0; i < 4; i++ {
		require.NoError(t, evidence.Append("inc", minute(10+i), value))
	}
	require.NoError(t, ChangeLog[string](s).Append("pod", minute(0), "c"))
	size := entrySize(t, "inc", value)
	policy := Policy{EvidenceCap: 2 * size, SizeCap: 1 << 30}

	result, err := NewCompactor(s, policy).Pass(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, result.Evicted)
	assert.Len(t, collect(t, evidence, "inc", minute(12), time.Time{}), 2)
	assert.Len(t, collect(t, ChangeLog[string](s), "pod",
		time.Time{}, time.Time{}), 1, "older changes stay")
}

func TestCompactorCanceledPassStopsBeforeWriting(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	changes := ChangeLog[string](s)
	require.NoError(t, changes.Append("pod", clock.Now().Add(-40*day), "c"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewCompactor(s, DefaultPolicy()).Pass(ctx)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Len(t, collect(t, changes, "pod", time.Time{}, time.Time{}), 1)
}

func TestCompactorRunPassesOnTicksAndStopsOnCancel(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	changes := ChangeLog[string](s)
	c := NewCompactor(s, DefaultPolicy())
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, tick) }()

	first := <-c.Passes()
	assert.Zero(t, first.Expired)
	require.NoError(t, changes.Append("pod", clock.Now().Add(-40*day), "c"))
	tick <- clock.Now()
	second := <-c.Passes()
	assert.Equal(t, 1, second.Expired)

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err, "cancellation is a clean stop")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

func TestCompactorFencedStoreDeletesNothing(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	changes := ChangeLog[string](s)
	require.NoError(t, changes.Append("pod", clock.Now().Add(-40*day), "c"))
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		return meta.Put(epochKey, writeUint(s.Epoch()+1))
	}))

	err := NewCompactor(s, DefaultPolicy()).Run(
		context.Background(), make(chan time.Time))

	assert.ErrorIs(t, err, ErrFenced)
	assert.Len(t, collect(t, changes, "pod", time.Time{}, time.Time{}), 1)
}
