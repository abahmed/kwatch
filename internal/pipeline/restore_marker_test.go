package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// A cold start that restores a marker from a run that stopped inside its
// startup window writes the marker back with what it carried: a second
// restart inside the window still owes the roll-ups, digest and listing.
func TestColdStartKeepsTheRestoredMarkerOnDisk(t *testing.T) {
	marker := announce.StartupState{
		Listing: announce.Listing{Key: "k", Incidents: []string{"a"}},
		Rollups: []announce.Listing{{Key: "r", Incidents: []string{"b"}}},
		Digest:  announce.DigestState{Listed: []string{"c"}},
	}
	store := &memStore{startup: &marker}
	h := newHarnessWith(t, time.Unix(1_800_000_000, 0),
		func(d *Dependencies) { d.Store = store })

	require.NoError(t, h.engine.restore())

	require.NotEmpty(t, store.startups, "the marker is saved on a cold start")
	saved := store.startups[len(store.startups)-1]
	assert.False(t, saved.Complete)
	assert.Equal(t, marker.Listing, saved.Listing)
	assert.Equal(t, marker.Rollups, saved.Rollups)
	assert.Equal(t, marker.Digest, saved.Digest)
}
