package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// watchSight is a fake of Config.Watching: the objects a detector still
// waits out a bad state on.
type watchSight struct{ watched map[inventory.EntityID]bool }

func (s *watchSight) watching(id inventory.EntityID) bool {
	return s.watched[id]
}

// restoredQuiet restores an announced pod incident with no member back,
// its grace bounded at 10 minutes.
func restoredQuiet(t *testing.T, cfg Config) *rig {
	t.Helper()
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	fresh := newRig(t, cfg)
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))
	return fresh
}

// Once the model is rebuilt, a restored incident whose findings did not
// come back recovers then, not at the upper bound of the grace.
func TestRestoredIncidentLeavesGraceOnceModelRebuilt(t *testing.T) {
	r := restoredQuiet(t, Config{})
	r.m.SettleGrace(at(2 * time.Minute))

	wantNone(t, r.tick(at(time.Minute)))
	assert.Equal(t, Open, r.only().State, "settle not over")
	assert.True(t, r.m.Rebuilding(at(time.Minute)))

	wantNone(t, r.tick(at(2*time.Minute)))
	assert.Equal(t, Recovering, r.only().State)
	assert.False(t, r.m.Rebuilding(at(2*time.Minute)))
	ds := r.tick(at(2*time.Minute + DefaultHold))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action, "resolved before the bound")
}

// A restored incident that reads a kind not synced yet (an optional
// kind) stays in the grace after the settle, until the kind syncs.
func TestRestoredIncidentWaitsForItsUnsyncedKind(t *testing.T) {
	sight := &kindSight{hidden: map[inventory.Kind]bool{kube.KindPod: true}}
	r := restoredQuiet(t, Config{Verifiable: sight.verifiable})
	r.m.SettleGrace(at(2 * time.Minute))

	wantNone(t, r.tick(at(3*time.Minute)))
	assert.Equal(t, Open, r.only().State, "pods are not listed yet")
	_, wake := r.m.Tick(at(3 * time.Minute))
	assert.LessOrEqual(t, wake, graceRecheck, "it looks again soon")

	sight.hidden = nil
	wantNone(t, r.tick(at(4*time.Minute)))
	assert.Equal(t, Recovering, r.only().State)
}

// A detector still waiting out a bad state on a former member (a
// sustain counted from when kwatch first saw it) keeps the incident in
// the grace: its finding is on its way back.
func TestRestoredIncidentWaitsWhileFormerMemberIsWatched(t *testing.T) {
	web := podSig("web").Entity
	sight := &watchSight{watched: map[inventory.EntityID]bool{web: true}}
	r := restoredQuiet(t, Config{Watching: sight.watching})
	r.m.SettleGrace(at(2 * time.Minute))

	wantNone(t, r.tick(at(5*time.Minute)))
	assert.Equal(t, Open, r.only().State, "its finding is coming back")

	r.raise(at(6*time.Minute), podSig("web"))
	wantNone(t, r.tick(at(6*time.Minute)))
	got := r.only()
	assert.Equal(t, Open, got.State)
	assert.Empty(t, got.Cycles, "a restart is not a recovery")
}

// The upper bound still holds: a wait that never ends, or a kind that
// never syncs, ends the grace at restoreGrace.
func TestRestoredIncidentGraceKeepsItsUpperBound(t *testing.T) {
	web := podSig("web").Entity
	sight := &watchSight{watched: map[inventory.EntityID]bool{web: true}}
	r := restoredQuiet(t, Config{Watching: sight.watching})
	r.m.SettleGrace(at(2 * time.Minute))

	wantNone(t, r.tick(at(10*time.Minute-time.Second)))
	assert.Equal(t, Open, r.only().State)
	wantNone(t, r.tick(at(10*time.Minute)))
	assert.Equal(t, Recovering, r.only().State)
}

// Without the settle, as when the sources never sync, the whole grace
// applies; a settle reported before the restore, or a second one, does
// not move it.
func TestSettleGraceNeedsRestoredIncidentsAndCountsOnce(t *testing.T) {
	r := newRig(t, Config{})
	r.m.SettleGrace(at(time.Minute))
	assert.True(t, r.m.graceSettled.IsZero(), "nothing restored")

	fresh := restoredQuiet(t, Config{})
	wantNone(t, fresh.tick(at(9*time.Minute)))
	assert.Equal(t, Open, fresh.only().State, "never synced")
	fresh.m.SettleGrace(at(9*time.Minute + 30*time.Second))
	fresh.m.SettleGrace(at(time.Minute))
	assert.Equal(t, at(9*time.Minute+30*time.Second), fresh.m.graceSettled)
}

// An incident whose member is back is not held by its own finding's
// onset: the wait counted on a member is that finding's.
func TestRebuiltIgnoresWaitsOnCurrentMembers(t *testing.T) {
	web := podSig("web").Entity
	sight := &watchSight{watched: map[inventory.EntityID]bool{web: true}}
	r := restoredQuiet(t, Config{Watching: sight.watching})
	r.m.SettleGrace(at(2 * time.Minute))
	r.raise(at(time.Minute), podSig("web"))
	r.tick(at(time.Minute))

	p := r.m.incidents[r.only().ID]
	assert.True(t, r.m.rebuilt(p, at(2*time.Minute)))
	assert.False(t, r.m.inGrace(p, at(2*time.Minute)))
}
