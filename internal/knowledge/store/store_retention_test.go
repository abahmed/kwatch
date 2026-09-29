package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompactRemovesOnlyOldValues(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: now}

	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Put old value
	clock.now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, s.Put(Decisions, "old", "value1"))

	// Put new value
	clock.now = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, s.Put(Decisions, "new", "value2"))

	// Compact at midnight between old and new
	cutoff := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	removed, err := s.Compact(Decisions, cutoff)
	require.NoError(t, err)

	assert.Equal(t, 1, removed)

	// Verify old is gone, new remains
	var val string
	found, _ := s.Get(Decisions, "old", &val)
	assert.False(t, found)

	found, _ = s.Get(Decisions, "new", &val)
	assert.True(t, found)
	assert.Equal(t, "value2", val)
}

func TestCompactBatchesWork(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: now}

	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Put old values that will all be compacted
	clock.now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		key := "old_" + string(byte(i))
		require.NoError(t, s.Put(Decisions, key, i))
	}

	// Put some new values
	clock.now = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, s.Put(Decisions, "new1", 9999))
	require.NoError(t, s.Put(Decisions, "new2", 9998))

	// Compact should work without errors
	cutoff := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	removed, err := s.Compact(Decisions, cutoff)
	require.NoError(t, err)

	// Should have removed some old values
	assert.Greater(t, removed, 0)

	// Verify new values still exist
	var val int
	found, _ := s.Get(Decisions, "new1", &val)
	assert.True(t, found)
	assert.Equal(t, 9999, val)
}

func TestCompactDoesNotRemoveCurrentCutoff(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &testClock{now: now}

	s, err := Open(path, Options{Now: clock.Now})
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	cutoff := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Put value exactly at cutoff
	clock.now = cutoff
	require.NoError(t, s.Put(Decisions, "at_cutoff", "value"))

	// Compact at cutoff
	removed, err := s.Compact(Decisions, cutoff)
	require.NoError(t, err)

	// Value at cutoff should not be removed (before, not <=)
	assert.Equal(t, 0, removed)

	var val string
	found, _ := s.Get(Decisions, "at_cutoff", &val)
	assert.True(t, found)
}

func TestSizeReturnsPositive(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	size, err := s.Size()
	require.NoError(t, err)
	assert.Greater(t, size, int64(0))
}

func TestSizeIncreaseWithData(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	size1, err := s.Size()
	require.NoError(t, err)

	// Add significant data - enough to force new pages
	largeValue := "this_is_a_large_value_repeated_many_times_" +
		"this_is_a_large_value_repeated_many_times_" +
		"this_is_a_large_value_repeated_many_times_"
	for i := 0; i < 500; i++ {
		require.NoError(t, s.Put(Decisions,
			"key_"+string(byte('a'+(i%26)))+
				"_"+string(byte('0'+(i/26)%10)), largeValue))
	}

	size2, err := s.Size()
	require.NoError(t, err)

	assert.Greater(t, size2, size1)
}

// testClock allows time control for tests
type testClock struct {
	now time.Time
}

func (tc *testClock) Now() time.Time {
	return tc.now
}
