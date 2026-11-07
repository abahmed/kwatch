package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/storage"
)

func TestDropLegacyPushRemovesTheRetiredKey(t *testing.T) {
	store, err := storage.Open(t.TempDir()+"/state.db",
		storage.Options{Now: func() time.Time { return time.Unix(1, 0) }})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.Claim()
	require.NoError(t, err)
	values := storage.StateValues[map[string]string](store)
	require.NoError(t, values.Put(legacyPushKey,
		map[string]string{"key": "old"}))
	require.NoError(t, values.Put("other", map[string]string{"k": "v"}))

	dropLegacyPush(store)
	dropLegacyPush(nil)

	_, ok, err := values.Get(legacyPushKey)
	require.NoError(t, err)
	require.False(t, ok, "the retired push key stays in the state file")
	_, ok, err = values.Get("other")
	require.NoError(t, err)
	require.True(t, ok, "other state was touched")
}
