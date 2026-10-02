package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func openDeferred(t *testing.T, path string, options Options) *Store {
	t.Helper()
	options.Now = newFakeClock().Now
	options.DeferRepair = true
	s, err := Open(path, options)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// A replica that has not yet confirmed the Lease must not delete the
// file: the reset waits for Claim.
func TestOpenDeferredKeepsUnusableFileUntilClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)
	writeVersion(t, path, SchemaVersion+1)
	before := fileBytes(t, path)

	s := openDeferred(t, path, Options{})

	pending, ok := s.Pending()
	require.True(t, ok)
	assert.Equal(t, ResetSchemaMismatch, pending.Reason)
	_, reset := s.Reset()
	assert.False(t, reset, "nothing was reset yet")
	assert.Equal(t, before, fileBytes(t, path))
	_, _, err := state(s).Get("k")
	assert.ErrorIs(t, err, ErrRepairPending)

	assert.Equal(t, uint64(1), claim(t, s))
	_, ok = s.Pending()
	assert.False(t, ok)
	done, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetSchemaMismatch, done.Reason)
	require.NoError(t, state(s).Put("k", "fresh"))
}

// Closing a handle that never claimed leaves an unusable file in place
// for the replica that does hold the Lease.
func TestOpenDeferredCloseWithoutClaimKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(path, []byte("not a bbolt file"), 0o600))

	s := openDeferred(t, path, Options{})
	pending, ok := s.Pending()
	require.NoError(t, s.Close())

	require.True(t, ok)
	assert.Equal(t, ResetUnreadable, pending.Reason)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "not a bbolt file", string(data))
}

func TestOpenDeferredRewritesOversizedFileAtClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	fillAndDelete(t, path, 200)
	before := fileBytes(t, path)

	s := openDeferred(t, path, Options{
		SizeCap:   64 << 10,
		FreeSpace: func(string) (uint64, error) { return 1 << 40, nil },
	})
	assert.Equal(t, before, fileBytes(t, path), "no rewrite before claim")
	got, found, err := state(s).Get("keep")
	require.NoError(t, err)
	assert.True(t, found, "an oversized file is readable before claim")
	assert.Equal(t, "v", got)

	assert.Equal(t, uint64(2), claim(t, s))
	assert.Less(t, fileBytes(t, path), before)
	got, _, err = state(s).Get("keep")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// A replica that has not claimed the store writes nothing at all, not
// even the schema version or the buckets of a healthy file.
func TestOpenDeferredWritesNothingBeforeClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)
	before := fileBytes(t, path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	s := openDeferred(t, path, Options{})
	got, found, err := state(s).Get("k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "old", got)
	require.ErrorIs(t, state(s).Put("k", "new"), ErrNotClaimed)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, fileBytes(t, path))
	assert.Equal(t, content, after, "the file is unchanged before claim")

	claim(t, s)
	require.NoError(t, state(s).Put("k", "new"))
}

func TestOpenDeferredDoesNotCreateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openDeferred(t, path, Options{})
	_, found, err := state(s).Get("k")
	require.NoError(t, err)
	assert.False(t, found)
	sizes, err := s.LogicalSize()
	require.NoError(t, err)
	assert.Empty(t, sizes)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "no file before claim")

	assert.Equal(t, uint64(1), claim(t, s))
	require.NoError(t, state(s).Put("k", "v"))
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

// A file written before a bucket existed is read without that bucket
// until Claim creates it.
func TestOpenDeferredReadsFileWithoutNewerBucket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(Threads))
	}))
	require.NoError(t, db.Close())

	s := openDeferred(t, path, Options{})
	threads := ThreadValues[string](s)
	_, found, err := threads.Get("t")
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, threads.Range("", func(string, string) error {
		t.Fatal("no thread was stored")
		return nil
	}))

	claim(t, s)
	require.NoError(t, threads.Put("t", "1"))
}
