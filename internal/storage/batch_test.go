package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogAppendAllKeepsOrder(t *testing.T) {
	l := TimelineLog[string](openClaimed(t, newFakeClock()))
	require.NoError(t, l.AppendAll([]Entry[string]{
		{Entity: "pod/a", At: minute(1), Value: "late"},
		{Entity: "pod/a", At: minute(0), Value: "first"},
		{Entity: "pod/a", At: minute(0), Value: "second"},
		{Entity: "pod/b", At: minute(0), Value: "other"},
	}))
	assert.Equal(t, []string{"first", "second", "late"},
		collect(t, l, "pod/a", time.Time{}, time.Time{}))
	require.NoError(t, l.AppendAll(nil))
}

func TestLogAppendAllRejectsBadEntityAtomically(t *testing.T) {
	l := TimelineLog[string](openClaimed(t, newFakeClock()))
	err := l.AppendAll([]Entry[string]{
		{Entity: "pod/a", At: minute(0), Value: "x"},
		{Entity: "bad\x00", At: minute(0), Value: "y"},
	})
	assert.ErrorIs(t, err, ErrBadEntity)
	assert.Empty(t, collect(t, l, "pod/a", time.Time{}, time.Time{}))
	assert.ErrorIs(t, l.AppendAll([]Entry[string]{{At: minute(0)}}),
		ErrEmptyKey)
}

func TestKeyedPutAllKeepsOtherKeys(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	k := BaselineValues[int](s)
	require.NoError(t, k.Put("kept", 1))
	require.NoError(t, k.PutAll(map[string]Item[int]{
		"a": {Value: 2}, "b": {Value: 3},
	}))
	for key, want := range map[string]int{"kept": 1, "a": 2, "b": 3} {
		got, ok, err := k.Get(key)
		require.NoError(t, err)
		require.True(t, ok, key)
		assert.Equal(t, want, got)
	}
	assert.ErrorIs(t, k.PutAll(map[string]Item[int]{"": {}}), ErrEmptyKey)
	require.NoError(t, k.PutAll(nil))
}
