package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A cause is news once: when it is new for the incident and has held for
// CauseHold over two evaluations. Flipping back to a cause already told,
// or to one that does not hold, changes nothing.
func TestCauseMemoryTellsEachCauseOnce(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var c causeMemory
	c.note("release", t0)
	require.Equal(t, "release", c.part("release"), "nothing told yet")
	c.record(c.part("release"))

	steps := []struct {
		name, cause string
		at          time.Duration
		want        string
	}{
		{"new, first evaluation", "startup", time.Minute, "release"},
		{"not held long enough", "startup", 70 * time.Second, "release"},
		{"held for CauseHold", "startup", 2 * time.Minute, "startup"},
	}
	for _, s := range steps {
		c.note(s.cause, t0.Add(s.at))
		assert.Equal(t, s.want, c.part(s.cause), s.name)
	}
	c.record(c.part("startup"))

	// Back and forth between causes already told: never news.
	for i, cause := range []string{"release", "startup", "release"} {
		at := t0.Add(time.Duration(10+i*10) * time.Minute)
		c.note(cause, at)
		c.note(cause, at.Add(CauseHold))
		assert.Equal(t, "startup", c.part(cause), cause)
	}
	// A flicker to a new cause that is gone by the next evaluation.
	c.note("node", t0.Add(time.Hour))
	c.note("release", t0.Add(time.Hour+time.Minute))
	assert.Equal(t, "startup", c.part("node"))
}

func TestCauseMemoryIsBounded(t *testing.T) {
	var c causeMemory
	for i := range 3 * toldCausesMax {
		c.record(string(rune('a' + i)))
	}
	assert.Len(t, c.told, toldCausesMax)
}

// The incident's fingerprint follows the rule, and what was told survives
// a restart.
func TestFlippingCauseIsNotAMaterialChange(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 1)
	r.raise(at(0), crashSig(pod))
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	release := &rootcause.CauseRecord{Rule: "release",
		Root: entity(kube.KindNode, "n1")}
	startup := &rootcause.CauseRecord{Rule: "startup-budget",
		Root: entity(kube.KindNode, "n2")}

	r.m.mu.Lock()
	p := r.m.lookup(deploy)
	settle := func(c *rootcause.CauseRecord, from time.Duration) string {
		p.Cause = c
		p.causes.note(causeIdentity(p), at(from))
		p.causes.note(causeIdentity(p), at(from+CauseHold))
		return fingerprint(p)
	}
	told := p.Digest
	first := settle(release, time.Hour)
	require.NotEqual(t, told, first, "a new cause that held is news")
	p.Digest = first
	p.causes.record(p.causePart())
	second := settle(startup, 2*time.Hour)
	require.NotEqual(t, first, second)
	p.Digest = second
	p.causes.record(p.causePart())
	assert.Equal(t, second, settle(release, 3*time.Hour),
		"back to a cause already told")
	r.m.mu.Unlock()

	fresh := newRig(t, Config{})
	fresh.m.Restore(r.m.Export(), time.Time{})
	fresh.m.mu.Lock()
	defer fresh.m.mu.Unlock()
	q := fresh.m.lookup(deploy)
	require.NotNil(t, q)
	assert.Equal(t, p.causes.told, q.causes.told)
	assert.Equal(t, p.causes.last, q.causes.last)
	assert.True(t, q.causes.known)
}

// The first cause found after none was told is news at once (it is no
// flip); going back to no cause is news once it holds, like any cause.
func TestCauseMemoryFirstCauseAndLostCause(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var c causeMemory
	c.record("")
	c.note("release", t0)
	assert.Equal(t, "release", c.part("release"), "first cause")
	c.record("release")
	c.note("", t0.Add(time.Minute))
	c.note("", t0.Add(time.Minute+CauseHold))
	assert.Equal(t, "", c.part(""), "back to no cause, held")
}

// A cause told, then gone long enough to tell "no cause", then back is
// a flip: it is not news again by itself.
func TestCauseMemoryToldCauseReturningIsNoNews(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	var c causeMemory
	c.note("release", t0)
	c.record(c.part("release"))
	c.note("", t0.Add(time.Minute))
	c.note("", t0.Add(time.Minute+CauseHold))
	require.Equal(t, "", c.part(""), "no cause held")
	c.record("")
	c.note("release", t0.Add(5*time.Minute))
	c.note("release", t0.Add(5*time.Minute+CauseHold))
	assert.Equal(t, "", c.part("release"), "told before")
}
