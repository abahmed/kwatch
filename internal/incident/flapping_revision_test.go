package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A flapping incident says nothing about a revised cause, so the owed
// update must not wait for the incident to resolve and come back: the
// resolve drops it.
func TestFlappingResolveClearsTheOwedRevision(t *testing.T) {
	r := newRig(t, Config{})
	runCycles(r, 2*time.Minute, 4*time.Minute, 19*time.Minute)
	require.Equal(t, Flapping, r.only().State)

	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node n1 is out of memory")
	r.apply(at(19*time.Minute+10*time.Second), detection.Changed, web)
	r.tick(at(19*time.Minute + 15*time.Second))
	require.True(t, r.m.Export()[0].Revised, "the revision is owed")

	r.clear(at(20*time.Minute), web)
	var resolved bool
	for now := 20 * time.Minute; now < 2*time.Hour; now += 10 * time.Second {
		for _, d := range r.tick(at(now)) {
			resolved = resolved || d.Action == Resolve
		}
	}
	require.True(t, resolved)
	assert.False(t, r.m.Export()[0].Revised,
		"a resolved incident owes nothing")
}

// A flapper that comes back and recovers again before its "failing
// again" update was sent never told the thread it was back: like a
// recovering incident, it closes without a second resolve message.
func TestFlapperThatRecoversUnheardResolvesNoSecondTime(t *testing.T) {
	r := newRig(t, Config{MaxHold: 2 * time.Minute,
		FlapWindow: 4 * time.Hour})
	web := podSig("web")
	now := time.Duration(0)
	settle := func(until time.Duration) bool {
		for ; now < until; now += 10 * time.Second {
			if hasAction(r.tick(at(now)), Resolve) {
				return true
			}
		}
		return false
	}
	for round := 0; ; round++ {
		r.raise(at(now), web)
		r.tick(at(now + time.Second))
		if p := r.only(); p.State == Flapping && p.Pending.ReopenOwed() {
			break
		}
		require.Less(t, round, 10, "the incident never flapped")
		// Fail long enough to be announced, so its resolve is sent.
		for end := now + 2*time.Minute; now < end; now += 10 * time.Second {
			r.tick(at(now))
		}
		r.clear(at(now), web)
		require.True(t, settle(now+time.Hour), "round %d resolves", round)
		now += time.Minute
	}

	now += 5 * time.Second
	r.clear(at(now), web)
	assert.False(t, settle(now+4*time.Hour),
		"a second resolve for a return nobody was told about")
	assert.Equal(t, Resolved, r.only().State)
}

func hasAction(ds []Decision, action Action) bool {
	for _, d := range ds {
		if d.Action == action {
			return true
		}
	}
	return false
}
