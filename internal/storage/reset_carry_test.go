package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
