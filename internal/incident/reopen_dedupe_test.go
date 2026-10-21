package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
)

// resolvedWebWithCycles announces web, lets it recover and resolve, and
// gives it earlier recoveries inside the flap window, so the number of
// cycles decides what its return does. It returns the time it resolved.
func resolvedWebWithCycles(
	t *testing.T, r *rig, web detection.Finding, earlier int,
) time.Time {
	t.Helper()
	announced(t, r, web)
	now := at(DefaultSettle + 10*time.Second)
	r.clear(now, web)
	r.tick(now)
	now = now.Add(DefaultHold + time.Second)
	require.Equal(t, Resolve, r.tick(now)[0].Action)
	r.m.mu.Lock()
	for _, p := range r.m.incidents {
		p.Cycles = nil
		for i := range earlier {
			p.Cycles = append(p.Cycles,
				now.Add(-time.Duration(earlier-i)*5*time.Minute))
		}
	}
	r.m.mu.Unlock()
	return now
}

// reopenOwed reports a "failing again" update the only incident owes.
func reopenOwed(r *rig) bool {
	p := r.only()
	return p.Pending.ReopenOwed()
}

// failingAgains counts the updates that tell a thread the failure is
// back, over ticks every ten seconds until end.
func failingAgains(r *rig, from, to time.Time) (again, flapping int) {
	for now := from; !now.After(to); now = now.Add(10 * time.Second) {
		for _, d := range r.tick(now) {
			switch d.Reason {
			case ReasonFailingAgain:
				again++
			case ReasonFlapping:
				flapping++
			}
		}
	}
	return again, flapping
}

// A flapping update that already says the incident is back leaves
// nothing for "failing again" to add: one message tells the thread, the
// owed one is not sent after it.
func TestFlappingUpdateTakesTheOwedFailingAgainWithIt(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	now := resolvedWebWithCycles(t, r, web, 1).Add(20 * time.Second)
	r.raise(now, web)
	require.Equal(t, Open, r.only().State, "setup: one more cycle to flap")
	require.True(t, reopenOwed(r))

	// It recovers and fails again before "failing again" is due.
	r.clear(now.Add(5*time.Second), web)
	r.tick(now.Add(5 * time.Second))
	back := now.Add(10 * time.Second)
	r.raise(back, web)
	again, flapping := failingAgains(r, back, back.Add(3*time.Minute))

	assert.Equal(t, 1, flapping, "the flapping update tells the thread")
	assert.Zero(t, again, "and the owed update is not sent after it")
	assert.False(t, reopenOwed(r))
}

// A reopen that flaps on its own owes "failing again", and a restart
// before it is sent must not lose it: the thread's last word is still
// the resolve.
func TestRestartKeepsTheFailingAgainOfAFlappingReopen(t *testing.T) {
	for _, restart := range []bool{false, true} {
		r := newRig(t, Config{})
		web := podSig("web")
		now := resolvedWebWithCycles(t, r, web, 2).Add(20 * time.Second)
		r.raise(now, web)
		require.Equal(t, Flapping, r.only().State)
		require.True(t, reopenOwed(r))

		if restart {
			now = now.Add(2 * time.Second)
			again := NewManager(Config{IDNonce: testNonce}, r.rule)
			again.Restore(r.m.Export(), now.Add(10*time.Minute))
			r.m = again
			r.active = map[detection.Key]detection.Finding{}
			now = now.Add(time.Minute)
			r.raise(now, web)
		}
		told, _ := failingAgains(r, now, now.Add(3*time.Minute))

		assert.Equal(t, 1, told, "restart=%v: the thread hears it", restart)
	}
}
