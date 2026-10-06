package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An incident opened after a restart is not a restored one: when it
// recovers it resolves after the hold, not after the restore grace of the
// incidents loaded from the state file.
func TestManagerRestoreGraceSparesIncidentsOpenedAfterRestart(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("old"))
	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	web := podSig("web")
	fresh.raise(at(time.Minute), web)
	wantAction(t, fresh.tick(at(time.Minute+DefaultSettle)),
		Announce, "settled")
	fresh.clear(at(2*time.Minute), web)
	wantNone(t, fresh.tick(at(2*time.Minute+time.Second)))

	ds := fresh.tick(
		at(2*time.Minute + time.Second + DefaultHold))
	wantAction(t, ds, Resolve, Reason("healthy for "+DefaultHold.String()))
}

// A restored recovering incident whose findings come back during the
// grace is the same failure continuing, not a new flap: a restart must
// not add a cycle to its flap count.
func TestRestoredRecoveringReturnIsNotAFlapCycle(t *testing.T) {
	web := podSig("web")
	old := newRig(t, Config{})
	announced(t, old, web)
	old.clear(at(2*time.Minute), web)
	old.tick(at(2 * time.Minute))
	require.Equal(t, Recovering, old.only().State)
	before := len(old.only().Cycles)

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(12*time.Minute))
	fresh.raise(at(3*time.Minute), web)
	fresh.tick(at(3*time.Minute + time.Second))

	got := fresh.only()
	assert.Equal(t, Open, got.State, "the failure is back")
	assert.Len(t, got.Cycles, before, "a restart is not a recovery")
}
