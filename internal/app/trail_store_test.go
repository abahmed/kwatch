package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/storage"
)

func openTrailStore(t *testing.T, path string) *storage.Store {
	t.Helper()
	s, err := storage.Open(path, storage.Options{Now: func() time.Time {
		return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	}})
	require.NoError(t, err)
	_, err = s.Claim()
	require.NoError(t, err)
	return s
}

func trailRecord(key, result string) delivery.TrailRecord {
	return delivery.TrailRecord{Version: 1, Key: key,
		Attempts: []delivery.SendAttempt{{
			At:     time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
			Result: result, Provider: "slack",
		}}}
}

// A trail written in one session is restored into the next session's
// delivery manager from the state file; the trail of an incident the
// file no longer holds is deleted at that load.
func TestTrailStoreRestoresAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openTrailStore(t, path)
	require.NoError(t, storage.IncidentRecords[map[string]string](s).
		Put("inc-1", map[string]string{"id": "inc-1"}))
	require.NoError(t, newOutboxStore(s).WriteTrails(
		[]delivery.TrailRecord{trailRecord("inc-1", "ok"),
			trailRecord("inc-gone", "failed")}, nil))
	require.NoError(t, s.Close())

	s = openTrailStore(t, path)
	defer func() { _ = s.Close() }()
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.AttachOutbox(newOutboxStore(s)))

	got := manager.SendTrail("inc-1")
	require.Len(t, got, 1)
	assert.Equal(t, "ok", got[0].Result)
	assert.Equal(t, "slack", got[0].Provider)
	assert.Nil(t, manager.SendTrail("inc-gone"))
	_, found, err := storage.TrailValues[delivery.TrailRecord](s).
		Get("inc-gone")
	require.NoError(t, err)
	assert.False(t, found, "a forgotten incident's trail is deleted")
}

// A state file with no saved trail loads an empty one.
func TestTrailStoreLoadsNothingFromFileWithoutTrails(t *testing.T) {
	records, err := newOutboxStore(openTestStore(t)).LoadTrails()
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestTrailStoreWriteRemovesNamedTrails(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, storage.IncidentRecords[string](s).Put("a", "x"))
	store := newOutboxStore(s)
	require.NoError(t, store.WriteTrails(
		[]delivery.TrailRecord{trailRecord("a", "ok")}, nil))
	require.NoError(t, store.WriteTrails(nil, []string{"a"}))
	records, err := store.LoadTrails()
	require.NoError(t, err)
	assert.Empty(t, records)
}
