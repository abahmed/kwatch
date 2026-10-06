package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A page that resolved reopens as the same incident after a restart
// inside RepageWindow: same ID, notify tier, no new page.
func TestReopenWindowSurvivesARestart(t *testing.T) {
	r, _ := pagedRig(t)
	id := r.only().ID
	resolved := resolvePage(t, r, 5*time.Minute)
	r.m.RecordPaged(id, false)
	recs := r.m.Export()

	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, time.Time{})
	raiseAt := resolved + 10*time.Minute
	fresh.raise(at(raiseAt), nodeDown())
	ds := tickEvery(fresh, raiseAt, raiseAt+DefaultSettle+time.Minute)

	require.Len(t, ds, 1)
	assert.Equal(t, id, ds[0].Incident.ID)
	assert.Equal(t, ReasonFailingAgain, ds[0].Reason)
	assert.Equal(t, Notify, ds[0].Incident.Tier)
	assert.Equal(t, 2, ds[0].Incident.RepeatCount)
}

// A "failing again" update still owed at a restart is sent exactly once.
func TestOwedReopenUpdateSurvivesARestart(t *testing.T) {
	r, _ := pagedRig(t)
	resolved := resolvePage(t, r, 5*time.Minute)
	raiseAt := resolved + 10*time.Minute
	r.raise(at(raiseAt), nodeDown())
	r.tick(at(raiseAt)) // reopened, update owed
	recs := r.m.Export()
	require.False(t, recs[0].ReopenedAt.IsZero())

	fresh := newRig(t, Config{})
	fresh.m.Restore(recs, at(raiseAt+10*time.Minute))
	fresh.raise(at(raiseAt+time.Minute), nodeDown())
	ds := tickEvery(fresh, raiseAt+time.Minute, raiseAt+12*time.Minute)

	again := 0
	for _, d := range ds {
		if d.Reason == ReasonFailingAgain {
			again++
		}
	}
	assert.Equal(t, 1, again)
}
