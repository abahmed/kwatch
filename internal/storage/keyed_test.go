package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type record struct {
	Name string
	Num  int
}

func TestKeyedBucketsRoundTrip(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	views := map[Bucket]Keyed[record]{
		Incidents:    IncidentRecords[record](s),
		Baselines:    BaselineValues[record](s),
		Fingerprints: FingerprintValues[record](s),
		State:        StateValues[record](s),
		Threads:      ThreadValues[record](s),
		Outbox:       OutboxValues[record](s),
	}
	for bucket, view := range views {
		want := record{Name: string(bucket), Num: len(bucket)}
		require.NoError(t, view.Put("key", want))

		got, found, err := view.Get("key")

		require.NoError(t, err)
		assert.True(t, found, bucket)
		assert.Equal(t, want, got, bucket)
	}
	_, found, err := IncidentRecords[record](s).Get("missing")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestKeyedBucketsAreSeparate(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	require.NoError(t, StateValues[string](s).Put("k", "state"))

	_, found, err := ThreadValues[string](s).Get("k")

	require.NoError(t, err)
	assert.False(t, found)
}

func TestKeyedPutReplacesAndDeleteRemoves(t *testing.T) {
	values := state(openClaimed(t, newFakeClock()))
	require.NoError(t, values.Put("k", "old"))
	require.NoError(t, values.Put("k", "new"))

	got, _, err := values.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "new", got)

	require.NoError(t, values.Delete("k"))
	require.NoError(t, values.Delete("k"), "deleting twice is fine")
	_, found, err := values.Get("k")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestKeyedRejectsEmptyKey(t *testing.T) {
	values := state(openClaimed(t, newFakeClock()))

	assert.ErrorIs(t, values.Put("", "v"), ErrEmptyKey)
	_, err := NewMirror(values).Replace(map[string]Item[string]{
		"": {Value: "v"},
	})
	assert.ErrorIs(t, err, ErrEmptyKey)
}

func TestKeyedRangeVisitsPrefixInKeyOrder(t *testing.T) {
	values := BaselineValues[int](openClaimed(t, newFakeClock()))
	for key, n := range map[string]int{
		"b/2": 4, "a/2": 2, "a/1": 1, "b/1": 3,
	} {
		require.NoError(t, values.Put(key, n))
	}
	var keys []string
	sum := 0

	err := values.Range("a/", func(key string, n int) error {
		keys = append(keys, key)
		sum += n
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"a/1", "a/2"}, keys)
	assert.Equal(t, 3, sum)
}

func TestKeyedRangeStopsOnVisitError(t *testing.T) {
	values := BaselineValues[int](openClaimed(t, newFakeClock()))
	require.NoError(t, values.Put("a", 1))
	require.NoError(t, values.Put("b", 2))
	stop := errors.New("stop")
	visited := 0

	err := values.Range("", func(string, int) error {
		visited++
		return stop
	})

	assert.ErrorIs(t, err, stop)
	assert.Equal(t, 1, visited)
}

func TestKeyedDeleteManyRemovesOnlyNamedKeys(t *testing.T) {
	values := BaselineValues[string](openClaimed(t, newFakeClock()))
	for _, key := range []string{"a", "b", "c"} {
		require.NoError(t, values.Put(key, key))
	}

	require.NoError(t, values.DeleteMany([]string{"a", "c", "missing"}))
	require.NoError(t, values.DeleteMany(nil), "an empty list is fine")

	assert.Equal(t, map[string]string{"b": "b"}, contents(t, values))
}

func TestKeyedExpiredValueIsHidden(t *testing.T) {
	clock := newFakeClock()
	values := IncidentRecords[string](openClaimed(t, clock))
	require.NoError(t, values.PutUntil("resolved", "r",
		clock.Now().Add(time.Hour)))
	require.NoError(t, values.Put("open", "o"))

	_, found, err := values.Get("resolved")
	require.NoError(t, err)
	assert.True(t, found, "not yet expired")

	clock.Advance(time.Hour)
	_, found, err = values.Get("resolved")
	require.NoError(t, err)
	assert.False(t, found, "expired values read as absent")
	var keys []string
	require.NoError(t, values.Range("", func(k, _ string) error {
		keys = append(keys, k)
		return nil
	}))
	assert.Equal(t, []string{"open"}, keys)
}
