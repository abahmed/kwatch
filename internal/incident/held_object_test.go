package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// objectSight is a kind that is capped: the kind as a whole is not
// verifiable, and only the objects in refused are out of sight.
type objectSight struct {
	refused map[inventory.EntityID]bool
	lost    bool
}

func (s *objectSight) config() Config {
	return Config{
		Verifiable: func(inventory.Kind) bool { return false },
		VerifiableObject: func(id inventory.EntityID) bool {
			return !s.lost && !s.refused[id]
		},
	}
}

func policySig() detection.Finding {
	id := inventory.NewEntityID("kyverno.io", "clusterpolicy", "", "labels")
	return sig(id, reasons.CustomResourceFailure, detection.Warning)
}

func heldNotes(p Incident) int {
	n := 0
	for _, e := range p.Timeline {
		if e.Text == heldNote {
			n++
		}
	}
	return n
}

func TestManagerResolvesObservedRootOfCappedKind(t *testing.T) {
	sight := &objectSight{}
	r := newRig(t, sight.config())
	policy := policySig()
	announced(t, r, policy)

	r.clear(at(5*time.Minute), policy)
	wantNone(t, r.tick(at(5*time.Minute)))
	require.Equal(t, Recovering, r.only().State)
	wantAction(t, r.tick(at(5*time.Minute+DefaultHold)), Resolve,
		"healthy for 3m0s")
	assert.Zero(t, heldNotes(r.only()))
}

func TestManagerHoldsRefusedRootAndNotesItOnce(t *testing.T) {
	policy := policySig()
	sight := &objectSight{
		refused: map[inventory.EntityID]bool{policy.Entity: true},
	}
	r := newRig(t, sight.config())
	announced(t, r, policy)

	r.clear(at(5*time.Minute), policy)
	for d := 5 * time.Minute; d < 5*24*time.Hour; d += time.Hour {
		wantNone(t, r.tick(at(d)))
	}
	assert.Equal(t, Open, r.only().State)
	assert.Equal(t, 1, heldNotes(r.only()))

	delete(sight.refused, policy.Entity)
	wantNone(t, r.tick(at(5*24*time.Hour)))
	wantAction(t, r.tick(at(5*24*time.Hour+DefaultHold)), Resolve,
		"healthy for 3m0s")
}

func TestManagerHoldsWhenAccessToTheKindIsLost(t *testing.T) {
	sight := &objectSight{}
	r := newRig(t, sight.config())
	policy := policySig()
	announced(t, r, policy)

	sight.lost = true
	r.clear(at(5*time.Minute), policy)
	wantNone(t, r.tick(at(5*time.Minute)))
	wantNone(t, r.tick(at(time.Hour)))
	assert.Equal(t, Open, r.only().State)
}

// Sight coming back changes no finding, so the held incident needs a
// timer of its own or it would wait for unrelated cluster activity.
func TestManagerHeldIncidentWakesToLookAgain(t *testing.T) {
	policy := policySig()
	sight := &objectSight{
		refused: map[inventory.EntityID]bool{policy.Entity: true},
	}
	r := newRig(t, sight.config())
	announced(t, r, policy)
	r.clear(at(5*time.Minute), policy)
	wantNone(t, r.tick(at(5*time.Minute)))

	_, next := r.m.Tick(at(6 * time.Minute))
	assert.Equal(t, heldRecheck, next)
}

func TestManagerRestoredRootOfCappedKindResolvesAfterGrace(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, policySig())

	sight := &objectSight{}
	fresh := newRig(t, sight.config())
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	grace := at(10 * time.Minute)
	wantNone(t, fresh.tick(at(9*time.Minute)))
	require.Equal(t, Open, fresh.only().State)

	wantNone(t, fresh.tick(grace))
	require.Equal(t, Recovering, fresh.only().State)
	wantAction(t, fresh.tick(grace.Add(DefaultHold)), Resolve,
		"healthy for 3m0s")
}

func TestManagerFlappingRootOfCappedKindResolvesAfterTheLongHold(
	t *testing.T,
) {
	sight := &objectSight{}
	r := newRig(t, sight.config())
	policy := policySig()
	announced(t, r, policy)
	now := at(DefaultSettle)
	for i := 0; r.only().State != Flapping; i++ {
		require.Less(t, i, 10, "the incident must start flapping")
		now = now.Add(time.Minute)
		r.clear(now, policy)
		r.tick(now)
		now = now.Add(time.Minute)
		r.raise(now, policy)
		r.tick(now)
	}

	now = now.Add(time.Minute)
	r.clear(now, policy)
	wantNone(t, r.tick(now))
	wantAction(t, r.tick(now.Add(DefaultMaxHold)), Resolve,
		Reason("stable for "+DefaultMaxHold.String()))
}
