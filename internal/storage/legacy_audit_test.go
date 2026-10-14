package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// A state file written by a release that still had the audit bucket
// opens, keeps its other data and loses the audit bucket.
func TestOpenDropsLegacyAuditBucket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStoreAt(t, path)
	claim(t, s)
	require.NoError(t, StateValues[string](s).Put("k", "v"))
	require.NoError(t, s.Close())
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(legacyAuditBucket)
		if err != nil {
			return err
		}
		return b.Put([]byte("old"), []byte("entry"))
	}))
	require.NoError(t, db.Close())

	s = openStoreAt(t, path)
	defer func() { _ = s.Close() }()
	claim(t, s)

	got, found, err := StateValues[string](s).Get("k")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "v", got)
	require.NoError(t, s.view(func(tx *bolt.Tx) error {
		assert.Nil(t, tx.Bucket(legacyAuditBucket))
		return nil
	}))
	assert.Empty(t, leftovers(t, path), "no reset for an old file")
}
