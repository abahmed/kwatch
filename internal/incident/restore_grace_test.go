package incident

import (
	"testing"
	"time"
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
	wantAction(t, ds, Resolve, "healthy for "+DefaultHold.String())
}
