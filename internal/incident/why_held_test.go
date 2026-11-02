package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

const cannotSeePolicy = "kwatch can't see clusterpolicy labels right " +
	"now, so it stays open until it can check again"

// wantCannotSee checks that the why of p says it is held for sight, with
// no hold or elapsed time that would reset every tick.
func wantCannotSee(t *testing.T, p Incident, now time.Time) {
	t.Helper()
	w := WhyOf(p, now)
	assert.True(t, w.CannotSee)
	assert.True(t, p.Journal.Timers.Held)
	assert.Equal(t, cannotSeePolicy, w.StillOpen)
	assert.Zero(t, w.HoldTotal)
	assert.Zero(t, w.HoldElapsed)
}

func TestWhyOfSaysAnOpenIncidentIsHeldForSight(t *testing.T) {
	policy := policySig()
	sight := &objectSight{
		refused: map[inventory.EntityID]bool{policy.Entity: true},
	}
	r := newRig(t, sight.config())
	announced(t, r, policy)
	r.clear(at(5*time.Minute), policy)
	r.tick(at(5 * time.Minute))
	require.Equal(t, Open, r.only().State)
	wantCannotSee(t, r.only(), at(6*time.Minute))

	delete(sight.refused, policy.Entity)
	r.tick(at(7 * time.Minute))
	w := WhyOf(r.only(), at(7*time.Minute))
	assert.False(t, w.CannotSee)
	assert.False(t, r.only().Journal.Timers.Held)
	assert.NotEqual(t, cannotSeePolicy, w.StillOpen)
}

func TestWhyOfSaysARecoveringIncidentIsHeldForSight(t *testing.T) {
	policy := policySig()
	sight := &objectSight{}
	r := newRig(t, sight.config())
	announced(t, r, policy)
	r.clear(at(5*time.Minute), policy)
	r.tick(at(5 * time.Minute))
	require.Equal(t, Recovering, r.only().State)

	sight.refused = map[inventory.EntityID]bool{policy.Entity: true}
	r.tick(at(6 * time.Minute))
	r.tick(at(7 * time.Minute))
	require.Equal(t, Recovering, r.only().State)
	wantCannotSee(t, r.only(), at(7*time.Minute))
}

func TestWhyOfSaysASettlingIncidentIsHeldForSight(t *testing.T) {
	policy := policySig()
	sight := &objectSight{
		refused: map[inventory.EntityID]bool{policy.Entity: true},
	}
	r := newRig(t, sight.config())
	r.raise(at(0), policy)
	r.clear(at(10*time.Second), policy)
	r.tick(at(20 * time.Second))
	require.Equal(t, Settling, r.only().State)
	wantCannotSee(t, r.only(), at(30*time.Second))
}
