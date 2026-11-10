package incident

import (
	"testing"
	"time"
)

// A clock stepped back an hour during the hold does not delay the first
// message by that hour: the hold counts from the corrected time.
func TestClockSteppedBackDuringTheHold(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	wantNone(t, r.tick(at(10*time.Second)))
	back := -time.Hour + 20*time.Second
	for d := back; d <= back+DefaultSettle+30*time.Second; d += 15 *
		time.Second {
		for _, dec := range r.tick(at(d)) {
			if dec.Action == Announce {
				return
			}
		}
	}
	t.Fatal("not announced within the hold after the clock stepped back")
}
