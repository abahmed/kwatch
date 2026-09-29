package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutGetDelete(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	type testVal struct {
		Name string
		Num  int
	}

	// Put and get
	original := testVal{Name: "test", Num: 42}
	err := s.Put(Decisions, "key1", original)
	require.NoError(t, err)

	var retrieved testVal
	found, err := s.Get(Decisions, "key1", &retrieved)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, original, retrieved)

	// Get non-existent
	found, err = s.Get(Decisions, "key2", &retrieved)
	require.NoError(t, err)
	assert.False(t, found)

	// Delete
	err = s.Delete(Decisions, "key1")
	require.NoError(t, err)

	found, err = s.Get(Decisions, "key1", &retrieved)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestPutReplacesValue(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	err := s.Put(Decisions, "key", "old")
	require.NoError(t, err)

	err = s.Put(Decisions, "key", "new")
	require.NoError(t, err)

	var got string
	found, err := s.Get(Decisions, "key", &got)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "new", got)
}

func TestForEachPrefix(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Put values with different prefixes
	require.NoError(t, s.Put(Decisions, "alpha/1", 1))
	require.NoError(t, s.Put(Decisions, "alpha/2", 2))
	require.NoError(t, s.Put(Decisions, "beta/1", 3))
	require.NoError(t, s.Put(Decisions, "beta/2", 4))

	// Iterate alpha prefix
	var keys []string
	err := s.ForEach(Decisions, "alpha/", func(key string,
		decode func(out any) error) error {
		keys = append(keys, key)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha/1", "alpha/2"}, keys)

	// Iterate beta prefix
	keys = nil
	err = s.ForEach(Decisions, "beta/", func(key string,
		decode func(out any) error) error {
		keys = append(keys, key)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta/1", "beta/2"}, keys)
}

func TestForEachOrder(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Put in non-alphabetical order
	require.NoError(t, s.Put(Decisions, "c", 3))
	require.NoError(t, s.Put(Decisions, "a", 1))
	require.NoError(t, s.Put(Decisions, "b", 2))

	// ForEach should return in key order
	var keys []string
	err := s.ForEach(Decisions, "", func(key string,
		decode func(out any) error) error {
		keys = append(keys, key)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestForEachDecodes(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	require.NoError(t, s.Put(Decisions, "key1", 10))
	require.NoError(t, s.Put(Decisions, "key2", 20))

	sum := 0
	err := s.ForEach(Decisions, "", func(key string,
		decode func(out any) error) error {
		var val int
		if err := decode(&val); err != nil {
			return err
		}
		sum += val
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 30, sum)
}

func TestAppendRange(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Append entries to a partition
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)

	require.NoError(t, s.Append(Changes, "pod-1", t1, "event1"))
	require.NoError(t, s.Append(Changes, "pod-1", t2, "event2"))
	require.NoError(t, s.Append(Changes, "pod-1", t3, "event3"))

	// Range all entries
	var events []string
	var times []int64
	err := s.Range(Changes, "pod-1", time.Time{}, time.Time{},
		func(at time.Time, decode func(out any) error) error {
			times = append(times, at.Unix())
			var event string
			if err := decode(&event); err != nil {
				return err
			}
			events = append(events, event)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"event1", "event2", "event3"}, events)
	assert.Equal(t, []int64{t1.Unix(), t2.Unix(), t3.Unix()}, times)
}

func TestRangeWithBounds(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)
	t4 := time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC)

	require.NoError(t, s.Append(Changes, "pod-1", t1, "e1"))
	require.NoError(t, s.Append(Changes, "pod-1", t2, "e2"))
	require.NoError(t, s.Append(Changes, "pod-1", t3, "e3"))
	require.NoError(t, s.Append(Changes, "pod-1", t4, "e4"))

	// Range from t2 to t3 (exclusive)
	var events []string
	err := s.Range(Changes, "pod-1", t2, t3,
		func(at time.Time, decode func(out any) error) error {
			var event string
			if err := decode(&event); err != nil {
				return err
			}
			events = append(events, event)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"e2"}, events)
}

func TestRangeZeroSinceMeansUnbounded(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC)

	require.NoError(t, s.Append(Changes, "pod-1", t1, "e1"))
	require.NoError(t, s.Append(Changes, "pod-1", t2, "e2"))

	// Range with zero since should include all
	var events []string
	err := s.Range(Changes, "pod-1", time.Time{}, t2,
		func(at time.Time, decode func(out any) error) error {
			var event string
			if err := decode(&event); err != nil {
				return err
			}
			events = append(events, event)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"e1"}, events)
}

func TestRangeZeroUntilMeansUnbounded(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC)

	require.NoError(t, s.Append(Changes, "pod-1", t1, "e1"))
	require.NoError(t, s.Append(Changes, "pod-1", t2, "e2"))
	require.NoError(t, s.Append(Changes, "pod-1", t3, "e3"))

	// Range with zero until should include all from t2 onwards
	var events []string
	err := s.Range(Changes, "pod-1", t2, time.Time{},
		func(at time.Time, decode func(out any) error) error {
			var event string
			if err := decode(&event); err != nil {
				return err
			}
			events = append(events, event)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, []string{"e2", "e3"}, events)
}

func TestUnknownCollectionError(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Try to access unknown collection
	_, err := s.Get("unknown_collection", "key", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownCollection)
}

func TestReplaceAllDeletesMissing(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	// Put initial values
	require.NoError(t, s.Put(Decisions, "a", 1))
	require.NoError(t, s.Put(Decisions, "b", 2))
	require.NoError(t, s.Put(Decisions, "c", 3))

	// Replace with subset
	err := s.ReplaceAll(Decisions, map[string]any{
		"a": 10,
		"c": 30,
	})
	require.NoError(t, err)

	// Verify a and c updated, b deleted
	var val int
	found, _ := s.Get(Decisions, "a", &val)
	assert.True(t, found)
	assert.Equal(t, 10, val)

	found, _ = s.Get(Decisions, "b", &val)
	assert.False(t, found)

	found, _ = s.Get(Decisions, "c", &val)
	assert.True(t, found)
	assert.Equal(t, 30, val)
}

func TestReplaceAllEmpty(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()
	require.NoError(t, s.Claim(1))

	require.NoError(t, s.Put(Decisions, "a", 1))
	require.NoError(t, s.Put(Decisions, "b", 2))

	// Replace with empty map deletes all
	err := s.ReplaceAll(Decisions, map[string]any{})
	require.NoError(t, err)

	var val int
	found, _ := s.Get(Decisions, "a", &val)
	assert.False(t, found)
	found, _ = s.Get(Decisions, "b", &val)
	assert.False(t, found)
}

// Helper to open a test store
func openTestStore(t *testing.T) *Store {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	return s
}
