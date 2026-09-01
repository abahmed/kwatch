package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func newSmartGroupingEngine() *Engine {
	return NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 60 * time.Second,
	})
}

// The first owner to fail in a namespace is announced at once; grouping only
// starts holding incidents back from the second owner, which is the earliest
// point a fan-out can be told apart from an isolated failure.
func TestSmartGroupingBuffersSameReason(t *testing.T) {
	e := newSmartGroupingEngine()
	_, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"the first owner alerts immediately",
	)
	assert.Equal(t, 1, len(e.state))
	_, action = e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	assert.Equal(
		t,
		model.ActionSkip,
		action,
		"the second owner is buffered — a fan-out may be starting",
	)
	assert.Equal(
		t,
		2,
		len(e.state),
		"buffered incidents are still added to state",
	)
	var hooks int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		hooks++
	}
	e.checkLifecycle()
	assert.Equal(t, 0, hooks, "no hooks before window expiry")
}

// A node going away made twelve deployments unready at once. The first owner
// alerts immediately; the rest are held for one window, and if enough owners
// fail the same way they collapse into a single namespace-wide alert that also
// absorbs the first one (closing its individual thread).
func TestNamespaceFanOutCollapsesIntoOneAlert(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newEngine := func(threshold int) *Engine {
		e := NewEngine(
			Config{
				Window:                   10 * time.Minute,
				SmartGroupingWindow:      60 * time.Second,
				NamespaceFanOutThreshold: threshold,
			},
		)
		e.now = mockClock(now)
		return e
	}
	type emitted struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	fail := func(
		e *Engine, ns string, owners ...string,
	) (direct []model.IncidentAction) {
		for _, o := range owners {
			_, a := e.Process(
				event.Event{
					PodName:   o + "-abc",
					Namespace: ns,
					Reason:    "ContainersNotReady",
				},
				o,
				nil,
			)
			if a != model.ActionSkip {
				direct = append(direct, a)
			}
		}
		return direct
	}
	collect := func(e *Engine) []emitted {
		var got []emitted
		e.config.LifecycleHook = func(
			inc *model.Incident, a model.IncidentAction,
		) {
			got = append(got, emitted{inc, a})
		}
		e.now = mockClock(now.Add(61 * time.Second))
		e.checkLifecycle()
		return got
	}
	groupCreates := func(got []emitted) (n int, last *model.Incident) {
		for _, g := range got {
			if IsGroupKey(g.inc.Key) && g.action == model.ActionCreate {
				n++
				last = g.inc
			}
		}
		return
	}

	// Six owners: the first alerts now; at window end, one alert for all six.
	e := newEngine(3)
	direct := fail(
		e,
		"dev",
		"readify",
		"api",
		"tracking",
		"accounts",
		"tdesk",
		"fleet",
	)
	assert.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate},
		direct,
		"only the first owner alerts before the window",
	)
	got := collect(e)
	n, group := groupCreates(got)
	require.Equal(t, 1, n, "a namespace-wide fan-out is one event, not six")
	assert.Equal(
		t,
		6,
		group.Count,
		"the collapsed alert must account for every owner, the first included",
	)
	var noted, falselyResolved bool
	for _, g := range got {
		if g.inc.Key == "dev:readify:ContainersNotReady:" {
			if g.action == model.ActionUpdate {
				noted = true
			}
			if g.action == model.ActionResolved {
				falselyResolved = true
			}
		}
	}
	assert.True(
		t,
		noted,
		"the first owner's thread gets a note pointing at the namespace-wide "+
			"alert",
	)
	assert.False(
		t,
		falselyResolved,
		"a pod that is still down must never be marked resolved",
	)

	// The first owner recovers later: its own thread gets the resolve.
	// (collect installed a hook bound to its own slice; rebind to ours.)
	got = nil
	e.config.LifecycleHook = func(
		inc *model.Incident, a model.IncidentAction,
	) {
		got = append(got, emitted{inc, a})
	}
	e.MarkResolved("dev:readify:ContainersNotReady:")
	var ownResolve bool
	for _, g := range got {
		if g.inc.Key == "dev:readify:ContainersNotReady:" &&
			g.action == model.ActionResolved {
			ownResolve = true
		}
	}
	assert.True(
		t,
		ownResolve,
		"the first owner resolves on its own thread when it actually recovers",
	)

	// Below the threshold: both owners are plain incidents.
	e2 := newEngine(3)
	direct2 := fail(e2, "dev", "readify", "api")
	got2 := collect(e2)
	assert.Len(t, direct2, 1, "first owner immediate")
	require.Len(t, got2, 1, "second owner released as itself at window end")
	assert.False(t, IsGroupKey(got2[0].inc.Key))
	assert.Equal(t, model.ActionCreate, got2[0].action)

	// Different namespaces do not merge with each other.
	e3 := newEngine(3)
	fail(e3, "dev", "a1", "a2", "a3")
	fail(e3, "prod", "b1", "b2", "b3")
	n3, _ := groupCreates(collect(e3))
	assert.Equal(t, 2, n3, "one collapsed alert per namespace")

	// The feature can be turned off: one notification per owner, none grouped.
	e4 := newEngine(0)
	direct4 := fail(
		e4,
		"dev",
		"readify",
		"api",
		"tracking",
		"accounts",
	)
	got4 := collect(e4)
	assert.Len(t, direct4, 1)
	assert.Len(
		t,
		got4,
		3,
		"the three buffered owners are each released as themselves",
	)
	for _, g := range got4 {
		assert.False(t, IsGroupKey(g.inc.Key))
	}
}

// Node-scoped group keys have three parts too ("reason|node|<name>"). Three
// different nodes failing the same way are three incidents, not one
// "namespace" called "node".
// A per-owner group that has been announced and is then absorbed by a
// namespace fan-out must have its thread closed, not left open forever.
//
// Through Process alone an owner-scoped buffer only ever holds one incident
// (the key is owner-scoped too), so an announced per-owner group is not
// reachable that way today. The fold still has to be correct: the flush state
// is seeded directly here, as any future path that produces one would leave it.
func TestNamespaceFanOutClosesTheGroupItAbsorbs(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(
		Config{
			Window:                   10 * time.Minute,
			SmartGroupingWindow:      60 * time.Second,
			NamespaceFanOutThreshold: 3,
		},
	)
	e.now = mockClock(now)
	type seen struct {
		key    model.IncidentKey
		action model.IncidentAction
	}
	var log []seen
	e.config.LifecycleHook = func(
		inc *model.Incident, a model.IncidentAction,
	) {
		log = append(log, seen{inc.Key, a})
	}

	// dep1's group was announced earlier and its thread is open.
	e.groupFlushStates["ContainersNotReady|ns|dep1"] = &groupFlushState{
		notified:       true,
		lastNotifiedAt: now.Add(-time.Hour),
		firstSeen:      now.Add(-time.Hour),
	}

	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		e.Process(
			event.Event{
				PodName:   dep + "-c",
				Namespace: "ns",
				Reason:    "ContainersNotReady",
			},
			dep,
			nil,
		)
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	var resolvedPerOwner, createdFanOut bool
	for _, s := range log {
		if s.key == "__group__:ContainersNotReady|ns|dep1" &&
			s.action == model.ActionResolved {
			resolvedPerOwner = true
		}
		if s.key == "__group__:ContainersNotReady|ns|*" &&
			s.action == model.ActionCreate {
			createdFanOut = true
		}
	}
	assert.True(
		t,
		resolvedPerOwner,
		"the absorbed per-owner group must be resolved, not orphaned",
	)
	assert.True(t, createdFanOut, "the namespace-wide alert takes over")
	_, still := e.groupFlushStates["ContainersNotReady|ns|dep1"]
	assert.False(t, still, "the absorbed group's flush state is forgotten")
}

func TestNamespaceFanOutDoesNotMergeNodeScopedGroups(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(
		Config{
			Window:                   10 * time.Minute,
			SmartGroupingWindow:      60 * time.Second,
			NamespaceFanOutThreshold: 3,
		},
	)
	e.now = mockClock(now)
	for _, node := range []string{"node-a", "node-b", "node-c"} {
		e.Process(
			event.Event{
				Resource: "node",
				PodName:  node,
				NodeName: node,
				Reason:   "DiskPressure",
			},
			node,
			nil,
		)
	}
	var got []*model.Incident
	e.config.LifecycleHook = func(
		inc *model.Incident, _ model.IncidentAction,
	) {
		got = append(got, inc)
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Len(
		t,
		got,
		3,
		"one alert per node; a node is not an owner in a namespace",
	)
	for _, inc := range got {
		assert.NotContains(
			t,
			string(inc.Key),
			"|*",
			"no fan-out key may be synthesised for node groups",
		)
	}
}

// A symptom suppressed under a mass failure is tracked, not dropped: it counts
// toward the mass failure, resolves silently, and — if still broken when
// the mass failure clears — is announced then instead of vanishing.
