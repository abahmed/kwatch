package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/storage"
)

func TestOutboxPendingCountsAndReportsUnknownOnFailure(t *testing.T) {
	store, err := storage.Open(t.TempDir()+"/state.db",
		storage.Options{Now: func() time.Time { return time.Unix(1, 0) }})
	require.NoError(t, err)
	_, err = store.Claim()
	require.NoError(t, err)
	require.NoError(t, storage.OutboxValues[json.RawMessage](store).
		Put("a", json.RawMessage(`{}`)))
	got := outboxPending(store)
	require.NotNil(t, got)
	require.Equal(t, 1, *got)

	require.NoError(t, store.Close())
	require.Nil(t, outboxPending(store),
		"a failed read must not report zero pending")
}
