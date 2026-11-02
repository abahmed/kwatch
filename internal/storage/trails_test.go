package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// A state file written before the trails bucket existed opens without a
// reset, keeps its data and gains an empty trails bucket.
func TestOpenAddsTrailsBucketToOlderFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, StateValues[string](s).Put("k", "v"))
	require.NoError(t, s.Close())
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(Trails))
	}))
	require.NoError(t, db.Close())

	s = openStoreAt(t, path)
	defer func() { _ = s.Close() }()
	claim(t, s)

	got, found, err := StateValues[string](s).Get("k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v", got)
	trails := TrailValues[[]string](s)
	_, found, err = trails.Get("inc-1")
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, trails.Put("inc-1", []string{"ok"}))
	got2, found, err := trails.Get("inc-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{"ok"}, got2)
	assert.Empty(t, leftovers(t, path), "no reset for an old file")
}
