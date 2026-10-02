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

func TestStoreClaimStartsAtOne(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()

	assert.Equal(t, uint64(1), claim(t, s))
	assert.Equal(t, uint64(1), s.Epoch())
}

func TestStoreClaimIncrementsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	first := openStoreAt(t, path)
	claim(t, first)
	require.NoError(t, first.Close())

	second := openStoreAt(t, path)
	defer second.Close()

	assert.Equal(t, uint64(2), claim(t, second))
	require.NoError(t, state(second).Put("key", "value"))
}

// A file claimed many times (for example under a Lease that has since
// been deleted and recreated) must stay claimable by the next holder.
func TestStoreClaimSucceedsAfterHighStoredEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s := openStoreAt(t, path)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(epochKey, writeUint(41))
	}))
	require.NoError(t, s.Close())

	s = openStoreAt(t, path)
	defer s.Close()

	assert.Equal(t, uint64(42), claim(t, s))
	require.NoError(t, state(s).Put("key", "value"))
}

func TestStoreClaimFailsOnClosedFile(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, s.Close())

	_, err := s.Claim()

	require.Error(t, err)
	assert.Zero(t, s.Epoch())
}

func TestStoreWriteBeforeClaimReturnsNotClaimed(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()

	err := state(s).Put("key", "test")

	assert.Equal(t, ErrNotClaimed, err)
}

// A stale writer keeps its old epoch in memory; once a later claim is
// stored, every write it attempts is fenced and nothing is written.
func TestStoreStaleWriterIsFencedAfterNewerClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	stale := openStoreAt(t, path)
	defer stale.Close()
	epoch := claim(t, stale)
	require.NoError(t, state(stale).Put("key1", "value1"))

	// A newer holder claims the same file.
	require.NoError(t, stale.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(epochKey, writeUint(epoch+1))
	}))

	err := state(stale).Put("key2", "value2")
	assert.Equal(t, ErrFenced, err)
	err = state(stale).Delete("key1")
	assert.Equal(t, ErrFenced, err)
	_, found, err := state(stale).Get("key2")
	require.NoError(t, err)
	assert.False(t, found)
}

// A fenced handle cannot be claimed again; a new term takes a new
// handle, which becomes the newest writer.
func TestStoreClaimNewRecoversFromFence(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	epoch := claim(t, s)
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put(epochKey, writeUint(epoch+1))
	}))
	require.ErrorIs(t, state(s).Put("k", "v"), ErrFenced)

	_, err := s.Claim()
	require.ErrorIs(t, err, ErrAlreadyClaimed)
	next, err := s.ClaimNew()

	require.NoError(t, err)
	assert.Equal(t, epoch+2, next.Epoch())
	require.NoError(t, state(next).Put("k", "v"))
	require.ErrorIs(t, state(s).Put("k", "w"), ErrFenced)
}

// A goroutine of an earlier term keeps its old handle. A new claim in
// the same process abandons that handle at once, so its writes fail
// even though the file stays open.
func TestStoreClaimNewAbandonsEarlierHandles(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	claim(t, s)
	second, err := s.ClaimNew()
	require.NoError(t, err)
	third, err := second.ClaimNew()
	require.NoError(t, err)

	assert.True(t, s.Abandoned())
	assert.True(t, second.Abandoned())
	assert.False(t, third.Abandoned())
	assert.ErrorIs(t, state(s).Put("k", "old"), ErrFenced)
	assert.ErrorIs(t, state(second).Delete("k"), ErrFenced)
	require.NoError(t, state(third).Put("k", "new"))
	value, found, err := state(s).Get("k")
	require.NoError(t, err)
	assert.True(t, found, "an abandoned handle can still read")
	assert.Equal(t, "new", value)
}

func TestStoreUnclaimedHandleCanRead(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	defer s.Close()
	writer, err := s.ClaimNew()
	require.NoError(t, err)
	require.NoError(t, state(writer).Put("k", "v"))

	value, found, err := state(s).Get("k")

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v", value)
	assert.ErrorIs(t, state(s).Put("k", "w"), ErrNotClaimed)
}

func TestStoreCloseRunsOnCloseOnceWhileWritable(t *testing.T) {
	s := openStoreAt(t, filepath.Join(t.TempDir(), "test.db"))
	claim(t, s)
	calls := 0
	s.OnClose(func() {
		calls++
		assert.NoError(t, state(s).Put("final", "v"))
	})

	require.NoError(t, s.Close())
	require.NoError(t, s.Close())

	assert.Equal(t, 1, calls)
}
