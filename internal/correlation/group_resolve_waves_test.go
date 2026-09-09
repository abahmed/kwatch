package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// A group that fills over several grouping windows flushes in waves. Every
// wave's members must stay tracked, or the members of earlier waves resolve
// one by one -- forty green ticks for one recovery.
func TestGroupResolveTrackerKeepsEarlierWaves(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var resolved []model.IncidentKey
	e := NewEngine(Config{
		Window:                   10 * time.Minute,
		SmartGroupingWindow:      60 * time.Second,
		NamespaceFanOutThreshold: 2,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			if action == model.ActionResolved {
				resolved = append(resolved, inc.Key)
			}
		},
	})
	e.now = mockClock(now)

	fail := func(pod, owner string) {
		e.processEvent(event.Event{
			PodName: pod, Namespace: "ns", Reason: "CrashLoopBackOff",
		}, owner, nil)
	}

	// Wave one: dep1 is announced on its own, dep2 and dep3 are buffered and
	// fan out into one namespace-wide group.
	fail("p1", "dep1")
	fail("p2", "dep2")
	fail("p3", "dep3")
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	// Wave two, in a fresh window: more owners join the same group.
	e.now = mockClock(now.Add(70 * time.Second))
	fail("p4", "dep4")
	fail("p5", "dep5")
	fail("p6", "dep6")
	e.now = mockClock(now.Add(131 * time.Second))
	e.checkLifecycle()

	// A wave-one member recovering must fold into the group, not announce.
	e.markResolved(BuildKey("ns", "dep2", "CrashLoopBackOff", ""))

	for _, key := range resolved {
		assert.NotEqual(
			t,
			BuildKey("ns", "dep2", "CrashLoopBackOff", ""),
			key,
			"a member from an earlier flush wave resolved on its own",
		)
	}

	e.mu.Lock()
	tracker := e.groupResolveTrackers["CrashLoopBackOff|ns|*"]
	e.mu.Unlock()
	if assert.NotNil(t, tracker, "the fan-out group keeps a tracker") {
		_, wave1 := tracker.members[BuildKey("ns", "dep3", "CrashLoopBackOff", "")]
		_, wave2 := tracker.members[BuildKey("ns", "dep5", "CrashLoopBackOff", "")]
		assert.True(t, wave1, "wave-one member still tracked")
		assert.True(t, wave2, "wave-two member tracked")
	}
}
