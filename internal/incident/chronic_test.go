package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A root that keeps re-opening within ChronicWindow waits one more
// doubling of the hold before it resolves, never beyond MaxHold.
func TestHoldForChronicFlapper(t *testing.T) {
	m := NewManager(Config{}, nil)
	now := at(time.Hour)
	calm := &Incident{Occurrences: []time.Time{at(0)}}
	chronic := &Incident{Occurrences: []time.Time{
		now.Add(-50 * time.Minute), now.Add(-30 * time.Minute),
		now.Add(-10 * time.Minute)}}

	assert.Equal(t, DefaultHold, m.holdFor(calm, now))
	assert.Equal(t, ChronicFactor*DefaultHold, m.holdFor(chronic, now))

	chronic.Cycles = []time.Time{now, now, now, now, now, now}
	assert.Equal(t, DefaultMaxHold, m.holdFor(chronic, now))
}

// Two opens inside ChronicWindow (one re-opening) make a chronic
// flapper; one does not, and opens older than the window never count.
func TestHoldForChronicNeedsOneRecentReopen(t *testing.T) {
	m := NewManager(Config{}, nil)
	now := at(3 * time.Hour)
	recent2 := []time.Time{now.Add(-20 * time.Minute), now}
	recent3 := []time.Time{now.Add(-40 * time.Minute),
		now.Add(-20 * time.Minute), now}
	stale := []time.Time{now.Add(-3 * time.Hour), now.Add(-2 * time.Hour),
		now.Add(-ChronicWindow - time.Minute), now}

	assert.Equal(t, DefaultHold,
		m.holdFor(&Incident{Occurrences: recent2[1:]}, now))
	assert.Equal(t, ChronicFactor*DefaultHold,
		m.holdFor(&Incident{Occurrences: recent2}, now))
	assert.Equal(t, ChronicFactor*DefaultHold,
		m.holdFor(&Incident{Occurrences: recent3}, now))
	assert.Equal(t, DefaultHold,
		m.holdFor(&Incident{Occurrences: stale}, now))
}

// The chronic doubling stacks on the recovery doubling and still stops
// at MaxHold.
func TestHoldForChronicStacksWithRecoveriesButCaps(t *testing.T) {
	m := NewManager(Config{}, nil)
	now := at(time.Hour)
	opens := []time.Time{now.Add(-40 * time.Minute),
		now.Add(-20 * time.Minute), now}
	one := &Incident{Occurrences: opens, Cycles: []time.Time{now}}
	assert.Equal(t, 2*ChronicFactor*DefaultHold, m.holdFor(one, now))

	many := &Incident{Occurrences: opens,
		Cycles: []time.Time{now, now, now, now}}
	assert.Equal(t, DefaultMaxHold, m.holdFor(many, now))
}

// A chronic flapper that stays healthy for MaxHold resolves: flapping
// never holds an incident open forever.
func TestChronicFlapperStillResolvesAfterMaxHold(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	runCycles(r, 2*time.Minute, 4*time.Minute, 30*time.Minute)
	require.Equal(t, Flapping, r.only().State)

	last := 30 * time.Minute
	r.clear(at(last), web)
	r.tick(at(last))
	wantNone(t, r.tick(at(last+DefaultMaxHold-time.Second)))
	wantAction(t, r.tick(at(last+DefaultMaxHold)), Resolve,
		"stable for 30m0s")
}

// A digest-tier incident that opened twice within DigestChronicWindow
// waits MaxHold: nobody is waiting for its "healthy again", so a
// recovery that does not last must not be announced as one. The window
// is longer than a notifying incident's, because node churn recurs
// hourly, not by the minute. A notifying incident keeps the usual hold.
func TestHoldForDigestChronicFlapper(t *testing.T) {
	m := NewManager(Config{}, nil)
	now := at(8 * time.Hour)
	opens := []time.Time{now.Add(-3 * time.Hour), now}

	digest := &Incident{Tier: Digest, Occurrences: opens}
	assert.Equal(t, DefaultMaxHold, m.holdFor(digest, now))

	once := &Incident{Tier: Digest, Occurrences: opens[1:]}
	assert.Equal(t, DefaultHold, m.holdFor(once, now))

	old := &Incident{Tier: Digest, Occurrences: []time.Time{
		now.Add(-DigestChronicWindow - time.Minute), now}}
	assert.Equal(t, DefaultHold, m.holdFor(old, now))

	notify := &Incident{Tier: Notify, Occurrences: opens}
	assert.Equal(t, DefaultHold, m.holdFor(notify, now))
}
