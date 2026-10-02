package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// writeVersion stores version in the file at path, as an older or newer
// kwatch would have.
func writeVersion(t *testing.T, path string, version uint64) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		return meta.Put(versionKey, writeUint(version))
	}))
	require.NoError(t, db.Close())
}

func leftovers(t *testing.T, path string) []string {
	t.Helper()
	names, err := filepath.Glob(path + ".*")
	require.NoError(t, err)
	return names
}

// seeded creates a claimed file at path holding one state value.
func seeded(t *testing.T, path string) {
	t.Helper()
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, state(s).Put("k", "old"))
	require.NoError(t, s.Close())
}

func TestOpenCurrentVersionDoesNotReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seeded(t, path)

	s := openStoreAt(t, path)
	defer s.Close()

	_, reset := s.Reset()
	assert.False(t, reset)
	got, found, err := state(s).Get("k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "old", got)
	assert.Empty(t, leftovers(t, path))
}

func TestOpenSchemaMismatchDeletesAndStartsFresh(t *testing.T) {
	for name, version := range map[string]uint64{
		"older": SchemaVersion - 1, "newer": SchemaVersion + 1,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			seeded(t, path)
			writeVersion(t, path, version)

			s := openStoreAt(t, path)
			defer s.Close()

			reset, ok := s.Reset()
			require.True(t, ok)
			assert.Equal(t, ResetSchemaMismatch, reset.Reason)
			assert.Equal(t, version, reset.OldVersion)
			assert.Empty(t, leftovers(t, path), "no backup is kept")
			assert.Equal(t, uint64(1), claim(t, s), "fresh epoch")
			_, found, err := state(s).Get("k")
			require.NoError(t, err)
			assert.False(t, found, "the fresh store is empty")
		})
	}
}

func TestOpenFileWithoutVersionIsSchemaMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucket([]byte("decisions"))
		return err
	}))
	require.NoError(t, db.Close())

	s := openStoreAt(t, path)
	defer s.Close()

	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetSchemaMismatch, reset.Reason)
	assert.Zero(t, reset.OldVersion)
}

func TestOpenUnreadableFileDeletesAndStartsFresh(t *testing.T) {
	for name, size := range map[string]int{
		"tiny": 15, "page sized": 16 << 10, "large": 64 << 10,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			garbage := make([]byte, size)
			for i := range garbage {
				garbage[i] = byte(i*7 + 1)
			}
			require.NoError(t, os.WriteFile(path, garbage, 0o600))

			s := openStoreAt(t, path)
			defer s.Close()

			reset, ok := s.Reset()
			require.True(t, ok)
			assert.Equal(t, ResetUnreadable, reset.Reason)
			assert.Empty(t, leftovers(t, path), "no backup is kept")
			claim(t, s)
			require.NoError(t, state(s).Put("k", "v"))
		})
	}
}

// A file held by another process is not bad data: Open must fail rather
// than rename the file from under its owner.
func TestOpenLockedFileFailsWithoutReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	holder := openStoreAt(t, path)
	defer holder.Close()
	previous := openTimeout
	openTimeout = 50 * time.Millisecond
	defer func() { openTimeout = previous }()

	_, err := Open(path, Options{Now: newFakeClock().Now})

	require.Error(t, err)
	assert.Empty(t, leftovers(t, path))
}

func TestOpenInaccessibleDirectoryFailsWithoutReset(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := Open(filepath.Join(file, "state.db"),
		Options{Now: newFakeClock().Now})

	require.Error(t, err)
}
