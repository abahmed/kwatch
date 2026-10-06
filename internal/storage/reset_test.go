package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
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

// assertKeptAside checks that the replaced file survives as the single
// path+".corrupt" copy.
func assertKeptAside(t *testing.T, path string) {
	t.Helper()
	assert.Equal(t, []string{path + corruptSuffix}, leftovers(t, path))
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

func TestOpenSchemaMismatchKeepsOldFileAndStartsFresh(t *testing.T) {
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
			assertKeptAside(t, path)
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

func TestOpenUnreadableFileKeepsOldFileAndStartsFresh(t *testing.T) {
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
			assertKeptAside(t, path)
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

// A second reset replaces the older quarantined file: only one is kept.
func TestSecondResetOverwritesQuarantinedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(path, []byte("first bad file"), 0o600))
	openStoreAt(t, path).Close()
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.WriteFile(path, []byte("second bad"), 0o600))
	openStoreAt(t, path).Close()

	assertKeptAside(t, path)
	kept, err := os.ReadFile(path + corruptSuffix)
	require.NoError(t, err)
	assert.Equal(t, "second bad", string(kept))
}

// An error that is not structural must never move a file aside.
func TestUnknownOpenErrorIsNotResettable(t *testing.T) {
	assert.False(t, resettable(errors.New("out of memory")))
	assert.True(t, resettable(berrors.ErrInvalid))
	assert.True(t, resettable(berrors.ErrChecksum))
}

func openWithVolumeLimit(t *testing.T, path string, limit int64) {
	t.Helper()
	s, err := Open(path, Options{
		Now: newFakeClock().Now, VolumeLimit: limit,
	})
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

// With a volume limit, a quarantined copy larger than its share would eat
// the budget the fresh file needs, so it is deleted.
func TestResetDeletesQuarantinedFileBeyondVolumeShare(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(path, make([]byte, 4096), 0o600))

	openWithVolumeLimit(t, path, 8192)

	assert.Empty(t, leftovers(t, path), "the oversized copy must go")
}

func TestResetKeepsSmallQuarantinedFileWithVolumeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(path, []byte("bad"), 0o600))

	openWithVolumeLimit(t, path, 1<<20)

	assertKeptAside(t, path)
}
