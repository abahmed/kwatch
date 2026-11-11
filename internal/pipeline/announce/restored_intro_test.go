package announce

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A restored incident a roll-up only named, that never spoke on its own,
// is still owed its introduction after a restart; one that spoke, or
// that no listing carried, is not.
func TestRestoredIncidentOwedItsIntroduction(t *testing.T) {
	r := &restartRig{t: t, rolledUp: map[string]bool{"api": true}}
	r.marker.Rollups = []Listing{{Key: "rollup/1",
		Incidents: []string{"inc-api", "inc-web"}}}
	r.start(foldNow, apiLost)

	owed := r.last.TakeRestoredIntros()
	require.Len(t, owed, 1)
	assert.Equal(t, "inc-api", owed[0].ID)
	assert.Empty(t, r.last.TakeRestoredIntros(), "handed over once")

	r.marker.Rollups[0].Followed = []string{"inc-api"}
	r.start(foldNow.Add(RestoredRelist), apiLost)
	assert.Empty(t, r.last.TakeRestoredIntros(),
		"its introduction went out before the restart")
}
