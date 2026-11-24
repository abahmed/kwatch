package announce

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
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
	r.start(foldNow.Add(time.Hour), apiLost)
	assert.Empty(t, r.last.TakeRestoredIntros(),
		"its introduction went out before the restart")
}

// A restored incident only the restored summary named is introduced
// once, not after every restart: the introduction sent is kept in the
// startup marker with the fingerprint it told, until the incident is
// over.
func TestRestoredIntroductionIsNotRepeatedByLaterRestarts(t *testing.T) {
	r := &restartRig{t: t, rolledUp: map[string]bool{"api": true},
		unheard: map[string]bool{"api": true},
		digests: map[string]string{"api": "d-api"}}
	r.start(foldNow, apiLost)
	owed := r.last.TakeRestoredIntros()
	require.Len(t, owed, 1)
	r.last.NoteSpoke(incident.Decision{Action: incident.Update,
		Reason: incident.ReasonIntroduced, Incident: owed[0]},
		notification.Message{})
	require.Contains(t, r.marker.Spoke, "inc-api")

	for i := 1; i <= 6; i++ {
		r.start(foldNow.Add(time.Duration(i)*time.Hour), apiLost)
		assert.Empty(t, r.last.TakeRestoredIntros(),
			"restart %d introduced it again", i)
	}
}

// A restored incident the restart only counted as unchanged was not
// named by it: it is owed no introduction, however often kwatch
// restarts.
func TestRestoredIncidentOnlyCountedIsOwedNoIntroduction(t *testing.T) {
	r := &restartRig{t: t, rolledUp: map[string]bool{"api": true},
		digests: map[string]string{"api": "d-api"}}
	for i := range 4 {
		r.start(foldNow.Add(time.Duration(i)*12*time.Hour), apiLost)
		assert.Empty(t, r.last.TakeRestoredIntros(),
			"restart %d introduced a counted incident", i)
		assert.Empty(t, r.last.ListedBy("inc-api"),
			"restart %d named a counted incident", i)
	}
}
