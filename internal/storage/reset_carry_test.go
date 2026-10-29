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

// seededQueue creates a claimed file holding a queued page, a thread id and
// an incident, then closes it.
func seededQueue(t *testing.T, path string) {
	t.Helper()
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, OutboxValues[string](s).Put("0001", "page"))
	require.NoError(t, ThreadValues[string](s).Put("inc-1", "ts-1"))
	require.NoError(t, IncidentRecords[string](s).Put("inc-1", "open"))
	require.NoError(t, s.Close())
}

func TestSchemaResetCarriesOutboxAndThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seededQueue(t, path)
	writeVersion(t, path, SchemaVersion+1)

	s := openStoreAt(t, path)
	defer s.Close()

	page, found, err := OutboxValues[string](s).Get("0001")
	require.NoError(t, err)
	assert.True(t, found, "a queued page must survive the reset")
	assert.Equal(t, "page", page)
	thread, found, err := ThreadValues[string](s).Get("inc-1")
	require.NoError(t, err)
	assert.True(t, found, "a thread id must survive the reset")
	assert.Equal(t, "ts-1", thread)
	_, found, err = IncidentRecords[string](s).Get("inc-1")
	require.NoError(t, err)
	assert.False(t, found, "other buckets start empty")
	reset, _ := s.Reset()
	assert.Equal(t, 2, reset.CarriedRecords)
}

func TestResetKeepsTwoTimestampedCopies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(path, []byte("bad file"), 0o600))
		openStoreAt(t, path).Close()
		require.NoError(t, os.Remove(path))
		time.Sleep(2 * time.Millisecond)
	}

	assert.Len(t, keptCopiesOf(path), keptCopies, "only the newest copies stay")
}

func TestUnreadablePageResetSalvagesIntactOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, OutboxValues[string](s).Put("0001", "page"))
	require.NoError(t, s.Close())
	pageSize, pages := seedPages(t, path)
	require.Greater(t, pages, int64(8))
	damagePage(t, path, pageSize, pages/2)

	s = openStoreAt(t, path)
	defer s.Close()

	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetUnreadable, reset.Reason)
	assert.Equal(t, 1, reset.CarriedRecords)
	page, found, err := OutboxValues[string](s).Get("0001")
	require.NoError(t, err)
	assert.True(t, found, "an intact queued page must survive")
	assert.Equal(t, "page", page)
}

func TestGarbageFileResetCarriesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	garbage := make([]byte, 64<<10)
	for i := range garbage {
		garbage[i] = byte(i*7 + 1)
	}
	require.NoError(t, os.WriteFile(path, garbage, 0o600))

	s := openStoreAt(t, path)
	defer s.Close()

	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetUnreadable, reset.Reason)
	assert.Zero(t, reset.CarriedRecords)
	assertKeptAside(t, path)
}

// A queued delivery the reset cannot read is lost: the reset says how
// many, and they count with the other undecodable values, so the
// corrupt-records metric shows it.
func TestResetCountsTheRecordsItCouldNotCarry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	seededQueue(t, path)
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(Outbox)).Put([]byte("0002"), []byte("??"))
	}))
	require.NoError(t, db.Close())
	writeVersion(t, path, SchemaVersion+1)

	s := openStoreAt(t, path)
	defer s.Close()

	reset, _ := s.Reset()
	assert.Equal(t, 2, reset.CarriedRecords)
	assert.Equal(t, 1, reset.SkippedRecords)
	assert.Equal(t, uint64(1), s.Stats().CorruptRecords)
}
