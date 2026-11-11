package storage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// A failed rewrite is not retried on every pass: the next try waits,
// the wait doubles up to its cap, and a success starts over.
func TestReclaimBacksOffAfterAFailedRewrite(t *testing.T) {
	smallReclaim(t)
	clock := newFakeClock()
	s := openClaimed(t, clock)
	fillAndEmpty(t, s, 1500)
	attempts := 0
	old := openCopy
	openCopy = func(path string) (*bolt.DB, error) {
		attempts++
		return nil, errors.New("cannot open")
	}
	t.Cleanup(func() { openCopy = old })

	_, err := s.Reclaim()
	require.NoError(t, err)
	require.Equal(t, 1, attempts)
	_, err = s.Reclaim()
	require.NoError(t, err)
	assert.Equal(t, 1, attempts, "retried inside the back-off")

	clock.Set(clock.Now().Add(reclaimRetryMin))
	_, err = s.Reclaim()
	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "no retry after the back-off")
	clock.Set(clock.Now().Add(reclaimRetryMin))
	_, _ = s.Reclaim()
	assert.Equal(t, 2, attempts, "the wait did not double")

	openCopy = old
	clock.Set(clock.Now().Add(reclaimRetryMax))
	rewrote, err := s.Reclaim()
	require.NoError(t, err)
	assert.True(t, rewrote)
	assert.Zero(t, s.retry.failures, "success resets the back-off")
}

func TestReclaimBackoffIsCapped(t *testing.T) {
	var b reclaimBackoff
	now := newFakeClock().Now()
	for range 20 {
		b.fail(now)
	}
	assert.Equal(t, now.Add(reclaimRetryMax), b.retryAt)
}
