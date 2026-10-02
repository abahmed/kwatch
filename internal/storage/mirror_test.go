package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contents reads a keyed bucket into a map.
func contents(t *testing.T, values Keyed[string]) map[string]string {
	t.Helper()
	got := map[string]string{}
	require.NoError(t, values.Range("", func(k, v string) error {
		got[k] = v
		return nil
	}))
	return got
}

func items(values map[string]string) map[string]Item[string] {
	out := make(map[string]Item[string], len(values))
	for k, v := range values {
		out[k] = Item[string]{Value: v}
	}
	return out
}

// The first Replace reads what is already on disk, so keys written by
// an earlier process are deleted and unchanged ones are kept.
func TestMirrorReplaceDeletesMissingKeys(t *testing.T) {
	values := FingerprintValues[string](openClaimed(t, newFakeClock()))
	require.NoError(t, values.Put("a", "1"))
	require.NoError(t, values.Put("b", "2"))
	require.NoError(t, values.Put("keep", "k"))

	diff, err := NewMirror(values).Replace(items(map[string]string{
		"a": "10", "c": "30", "keep": "k",
	}))

	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 2, Deleted: 1}, diff)
	assert.Equal(t, map[string]string{"a": "10", "c": "30", "keep": "k"},
		contents(t, values))
}

// A short value is corrupt; it must still be deleted when no longer named.
func TestMirrorReplaceDeletesCorruptShortValues(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	values := FingerprintValues[string](s)
	putRaw(t, s, Fingerprints, []byte("bad"), []byte("short"))
	putRaw(t, s, Fingerprints, []byte("rewritten"), []byte("short"))

	diff, err := NewMirror(values).Replace(items(map[string]string{
		"rewritten": "ok",
	}))

	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 1, Deleted: 1}, diff)
	assert.Equal(t, map[string]string{"rewritten": "ok"},
		contents(t, values))
}

func TestMirrorReplaceWritesOnlyChangedKeys(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	values := FingerprintValues[string](s)
	mirror := NewMirror(values)
	snapshot := map[string]string{"a": "1", "b": "2", "c": "3"}
	_, err := mirror.Replace(items(snapshot))
	require.NoError(t, err)

	before := s.counters.writeTxs.Load()
	diff, err := mirror.Replace(items(snapshot))
	require.NoError(t, err)
	assert.Equal(t, Diff{}, diff)
	assert.Equal(t, before, s.counters.writeTxs.Load(),
		"an unchanged snapshot opens no write transaction")

	snapshot["b"] = "20"
	delete(snapshot, "c")
	diff, err = mirror.Replace(items(snapshot))

	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 1, Deleted: 1}, diff)
	assert.Equal(t, map[string]string{"a": "1", "b": "20"},
		contents(t, values))
}

// A changed expiry is a change even when the value is the same.
func TestMirrorReplaceWritesChangedExpiry(t *testing.T) {
	clock := newFakeClock()
	values := IncidentRecords[string](openClaimed(t, clock))
	mirror := NewMirror(values)
	_, err := mirror.Replace(map[string]Item[string]{"i": {Value: "r"}})
	require.NoError(t, err)

	diff, err := mirror.Replace(map[string]Item[string]{
		"i": {Value: "r", Expires: clock.Now().Add(time.Hour)},
	})

	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 1}, diff)
}

// The compactor deletes expired values behind the mirror's back. The
// mirror must notice and write the value again if the caller still
// holds it with a new expiry.
func TestMirrorReplaceRereadsAfterOtherWrites(t *testing.T) {
	clock := newFakeClock()
	s := openClaimed(t, clock)
	values := IncidentRecords[string](s)
	mirror := NewMirror(values)
	soon := clock.Now().Add(time.Minute)
	_, err := mirror.Replace(map[string]Item[string]{
		"i": {Value: "r", Expires: soon},
	})
	require.NoError(t, err)
	clock.Advance(time.Hour)
	_, err = NewCompactor(s, DefaultPolicy()).Pass(context.Background())
	require.NoError(t, err)
	require.Empty(t, contents(t, values), "the compactor deleted it")

	diff, err := mirror.Replace(map[string]Item[string]{
		"i": {Value: "r", Expires: soon},
	})

	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 1}, diff, "the deletion was noticed")
	_, found, err := values.Get("i")
	require.NoError(t, err)
	assert.False(t, found, "written again, but still expired")
	diff, err = mirror.Replace(map[string]Item[string]{
		"i": {Value: "r", Expires: soon},
	})
	require.NoError(t, err)
	assert.Equal(t, Diff{}, diff)
}

// A failed write must not be remembered as written: the next Replace on
// a working handle writes everything that is still different.
func TestMirrorReplaceFailureIsNotRemembered(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	stale := NewMirror(FingerprintValues[string](s))
	next, err := s.ClaimNew()
	require.NoError(t, err)

	_, err = stale.Replace(items(map[string]string{"a": "1"}))
	require.ErrorIs(t, err, ErrFenced)
	values := FingerprintValues[string](next)
	assert.Empty(t, contents(t, values))

	diff, err := NewMirror(values).Replace(items(map[string]string{
		"a": "1",
	}))
	require.NoError(t, err)
	assert.Equal(t, Diff{Put: 1}, diff)
}
