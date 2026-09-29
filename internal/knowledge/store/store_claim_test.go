package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestOpenRequiresClock(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")

	_, err := Open(path, Options{Now: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clock is required")
}

func TestOpenCreatesDirectoryWithRightPermissions(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "subdir", "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	// Check directory was created
	info, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	// Check file was created with right permissions
	fileInfo, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
}

func TestOpenWritesSchemaVersion(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	s.Close()

	// Reopen and verify version is stored
	s2, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s2.Close()

	// Verify it doesn't fail (version was written and read successfully)
}

func TestOpenNewerSchemaError(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create store with current version
	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)

	// Manually bump the version in the DB
	err = s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		return meta.Put(versionKey, writeUint(SchemaVersion+1))
	})
	require.NoError(t, err)
	s.Close()

	// Reopening should fail
	_, err = Open(path, Options{Now: func() time.Time { return now }})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNewerSchema),
		"got %v", err)
}

func TestClaimRecordsEpoch(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	err = s.Claim(5)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), s.epoch)
}

func TestClaimLowerEpochAfterHigherReturnsFenced(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	// First holder claims 7
	err = s.Claim(7)
	require.NoError(t, err)

	// Later holder tries to claim 5
	err = s.Claim(5)
	require.Error(t, err)
	assert.Equal(t, ErrFenced, err)
}

func TestClaimHigherEpochAfterLowerSucceeds(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	// First holder claims 5
	err = s.Claim(5)
	require.NoError(t, err)

	// Later holder claims 7
	err = s.Claim(7)
	require.NoError(t, err)
	assert.Equal(t, uint64(7), s.epoch)
}

func TestWriteBeforeClaimReturnError(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	// Try to write without claiming
	err = s.Put(Decisions, "key", map[string]string{"value": "test"})
	require.Error(t, err)
	assert.Equal(t, ErrNotClaimed, err)
}

func TestWriteAfterClaimSucceeds(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	err = s.Claim(5)
	require.NoError(t, err)

	err = s.Put(Decisions, "key", map[string]string{"value": "test"})
	require.NoError(t, err)
}

func TestFencingPreventsWrites(t *testing.T) {
	tempdir := t.TempDir()
	path := filepath.Join(tempdir, "test.db")
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	s, err := Open(path, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()

	// First holder claims and writes
	err = s.Claim(5)
	require.NoError(t, err)
	err = s.Put(Decisions, "key1", "value1")
	require.NoError(t, err)

	// Second holder claims
	err = s.Claim(7)
	require.NoError(t, err)

	// First holder (now fenced) tries to write
	s.epoch = 5 // Force s.epoch back to simulate stale holder
	err = s.Put(Decisions, "key2", "value2")
	require.Error(t, err)
	assert.Equal(t, ErrFenced, err)
}
