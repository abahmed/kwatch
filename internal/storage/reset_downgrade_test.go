package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
	"k8s.io/klog/v2"
)

// readVersionAt reads the schema version stored in the file at path.
func readVersionAt(t *testing.T, path string) uint64 {
	t.Helper()
	db, err := bolt.Open(path, 0o600, &bolt.Options{
		Timeout: time.Second, ReadOnly: true})
	require.NoError(t, err)
	defer db.Close()
	var version uint64
	require.NoError(t, db.View(func(tx *bolt.Tx) error {
		version = readUint(tx.Bucket(metaBucket).Get(versionKey))
		return nil
	}))
	return version
}

// A rollback to an older kwatch opens a file written by a newer one. The
// file is kept, not deleted, and only the outbox and the thread ids come
// across: open incidents and the rest start empty and are rebuilt from the
// cluster.
func TestDowngradeKeepsNewerFileAndCarriesOnlyQueueAndThreads(t *testing.T) {
	out := captureLog(t)
	path := filepath.Join(t.TempDir(), "state.db")
	seededQueue(t, path)
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, state(s).Put("k", "newer"))
	require.NoError(t, s.Close())
	writeVersion(t, path, SchemaVersion+1)

	s = openStoreAt(t, path)
	defer s.Close()
	klog.Flush()

	reset, ok := s.Reset()
	require.True(t, ok)
	assert.Equal(t, ResetSchemaMismatch, reset.Reason,
		"the reason that kwatch_storage_resets_total counts")
	assert.Equal(t, uint64(SchemaVersion+1), reset.OldVersion)
	assert.Equal(t, 2, reset.CarriedRecords)
	assert.Zero(t, reset.SkippedRecords)

	kept := keptCopiesOf(path)
	require.Len(t, kept, 1, "the newer file must be kept aside")
	assert.Equal(t, uint64(SchemaVersion+1), readVersionAt(t, kept[0]),
		"the kept copy is the untouched newer file")
	assert.Contains(t, out.String(), "keptAs="+`"`+kept[0]+`"`)
	assert.Contains(t, out.String(), "reason=\"schema_mismatch\"")

	_, found, err := OutboxValues[string](s).Get("0001")
	require.NoError(t, err)
	assert.True(t, found, "queued alerts are carried")
	_, found, err = ThreadValues[string](s).Get("inc-1")
	require.NoError(t, err)
	assert.True(t, found, "chat thread ids are carried")
	_, found, err = IncidentRecords[string](s).Get("inc-1")
	require.NoError(t, err)
	assert.False(t, found, "open incidents are NOT carried")
	_, found, err = state(s).Get("k")
	require.NoError(t, err)
	assert.False(t, found, "other state is NOT carried")
}
