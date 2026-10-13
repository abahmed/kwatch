package pipeline

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/storage"
)

// rc17Incidents are records as rc.17 wrote them: no Place on the cause,
// no Mode history, and reasons and modes that no detector raises now.
const rc17Incidents = `[
 {"ID":"old-open","Root":{"Kind":"deployment","Namespace":"shop",
  "Name":"api"},"Tier":2,"State":1,"Mode":"Cert.Expiring",
  "RootReasons":["TLSCertExpiringSoon"],
  "Cause":{"Rule":"image-change","Root":{"Kind":"deployment",
   "Namespace":"shop","Name":"api"}}},
 {"ID":"old-quota","Root":{"Kind":"namespace","Name":"shop"},
  "Tier":2,"State":1,"Mode":"Quota.NearLimit",
  "RootReasons":["ResourceQuotaNearLimit"]}
]`

// rc17Startup is a startup marker whose digest state has no record of
// what the digests listed.
const rc17Startup = `{"Complete":true,"Digest":{"Since":
 "2026-10-01T00:00:00Z","Listed":["old-open"]}}`

// dropBuckets removes top-level buckets from the closed file, as a file
// written before they existed lacks them.
func dropBuckets(t *testing.T, path string, names ...storage.Bucket) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		for _, name := range names {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}))
	require.NoError(t, db.Close())
}

// A state file written by rc.17 restores cleanly: it has no changes, no
// healthy marks, no flight recording, no cause-check or unexplained
// counts, no record of what the digests listed, and incidents with
// retired reasons and no cause place. Nothing is an error and everything
// that is there comes back.
func TestRc17StateFileRestoresCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	start := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	seed, err := storage.Open(path, storage.Options{Now: func() time.Time {
		return start
	}})
	require.NoError(t, err)
	_, err = seed.Claim()
	require.NoError(t, err)
	var records []incident.Record
	require.NoError(t, json.Unmarshal([]byte(rc17Incidents), &records))
	var marker announce.StartupState
	require.NoError(t, json.Unmarshal([]byte(rc17Startup), &marker))
	old := NewIncidentStore(seed)
	require.NoError(t, old.SaveIncidents(records))
	require.NoError(t, old.SaveStartup(marker))
	require.NoError(t, seed.Close())
	dropBuckets(t, path, storage.Changes, storage.Flight)

	s, err := storage.Open(path, storage.Options{Now: func() time.Time {
		return start.Add(time.Hour)
	}})
	require.NoError(t, err)
	_, err = s.Claim()
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	h := newHarnessWith(t, start.Add(time.Hour), func(d *Dependencies) {
		d.Store = NewIncidentStore(s)
	})
	require.NoError(t, h.engine.restore())
	history := h.engine.storage.history
	history.start()
	t.Cleanup(func() { history.stop(time.After(time.Second)) })

	assert.Len(t, h.engine.deps.Incidents.Incidents(), 2,
		"both records restore, retired reasons and all")
	for _, p := range h.engine.deps.Incidents.Incidents() {
		if p.Cause != nil {
			assert.Nil(t, p.Cause.Place, "no place was recorded")
		}
	}
	assert.Empty(t, h.engine.announcer.collect.Startup.Summary.Digest.Shown,
		"the digest lists everything again")
	report := h.engine.Status()
	assert.Empty(t, report.CauseChecks, "no cause-check counts")
	assert.Zero(t, report.Unexplained.Events+report.Unexplained.Incidents,
		"no unexplained counts")
	changes, err := h.engine.deps.Store.(changeStore).LoadChanges(
		time.Time{}, 10)
	require.NoError(t, err)
	assert.Empty(t, changes)
	marks, err := h.engine.deps.Store.(healthyStore).LoadHealthy(10)
	require.NoError(t, err)
	assert.Empty(t, marks)
}
