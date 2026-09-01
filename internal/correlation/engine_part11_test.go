package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSmartGroupingFoldRekeysAllMembers(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Both members fold → both migrate to folded keys. The group still tracks
	// them (the loops are ongoing), so it must NOT resolve.
	cs := &model.ContainerState{RestartCount: 6}
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		cs,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		cs,
	)
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions,
		"folding must not resolve the group synchronously")

	e.now = mockClock(now.Add(90 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions,
		"folding must not resolve the group on the next tick either")
}

func TestResolveByResourceReleasesGroupMember(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// Resolving one member via ResolveByResource must not emit a group
	// resolve until every member has resolved.
	e.ResolveByResource("pod", "dep2")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate},
		actions,
		"group not fully resolved yet",
	)

	e.ResolveByResource("pod", "dep1")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)
}

func TestSmartGroupingNewOccurrenceCreatesAgain(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var actions []model.IncidentAction
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			actions = append(actions, action)
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	// All members resolve → batch group resolve resets the flush state.
	e.MarkResolved("ns:dep1:CrashLoopBackOff:")
	e.MarkResolved("ns:dep2:CrashLoopBackOff:")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)

	// A new occurrence of the same group (after the member cooldown) must
	// CREATE again rather than updating the previously-resolved key.
	e.now = mockClock(now.Add(11*time.Minute + 2*time.Second))
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p4",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(12*time.Minute + 5*time.Second))
	e.checkLifecycle()

	require.Len(t, actions, 3)
	assert.Equal(t, model.ActionCreate, actions[2])
}

// Reason-adaptive scope tests.

func TestSmartGroupingOwnerScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"},
		"dep2",
		nil,
	)
	e.mu.Lock()
	_, has1 := e.groupBuffers["OOMKilled|ns|dep1"]
	_, has2 := e.groupBuffers["OOMKilled|ns|dep2"]
	e.mu.Unlock()
	assert.False(t, has1, "the first owner is announced, not buffered")
	assert.True(
		t,
		has2,
		"the second owner is buffered under its own owner-scoped key",
	)
}

func TestSmartGroupingNodeScope(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{
			PodName:  "node-1",
			Resource: "node",
			NodeName: "node-1",
			Reason:   "DiskPressure",
		},
		"node-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:  "node-2",
			Resource: "node",
			NodeName: "node-2",
			Reason:   "DiskPressure",
		},
		"node-2",
		nil,
	)

	e.mu.Lock()
	_, has1 := e.groupBuffers["DiskPressure|node|node-1"]
	_, has2 := e.groupBuffers["DiskPressure|node|node-2"]
	e.mu.Unlock()
	assert.True(t, has1, "node-1 group must exist")
	assert.True(t, has2, "node-2 group must exist")
}

func TestSmartGroupingSignatureScope(t *testing.T) {
	e := newSmartGroupingEngine()
	sigLog := "connection refused:5432"
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns1",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns2",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	gk := "CrashLoopBackOff|sig|Postgres unreachable — check the DB " +
		"Service/endpoints + connection string."
	e.mu.Lock()
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "signature-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries), "both owners in same signature group")
}

func TestSmartGroupingSignatureFallback(t *testing.T) {
	e := newSmartGroupingEngine()
	// No logs set → no signature match → owner-scoped fallback
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	e.mu.Lock()
	_, has1 := e.groupBuffers["CrashLoopBackOff|ns|dep1"]
	_, has2 := e.groupBuffers["CrashLoopBackOff|ns|dep2"]
	_, hasSig := e.groupBuffers["CrashLoopBackOff|sig|"]
	e.mu.Unlock()
	assert.False(
		t,
		has1,
		"the first owner is announced immediately, not buffered",
	)
	assert.True(t, has2, "dep2 falls back to its owner-scoped key")
	assert.False(t, hasSig, "no signature-scoped group for empty logs")
}

func TestSmartGroupingImagePerImage(t *testing.T) {
	e := newSmartGroupingEngine()
	msg := "not found: nginx:latest"
	ev := event.Event{
		PodName: "p1", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|img|nginx:latest|ns|ns"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "image-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries))
}

func TestSmartGroupingImageGlobal(t *testing.T) {
	e := newSmartGroupingEngine()
	msg := "toomanyrequests: rate limit"
	ev := event.Event{
		PodName: "p1", Namespace: "ns1", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.Process(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns2", Reason: "ImagePullBackOff",
		Image: "alpine:latest", Message: msg,
	}
	e.Process(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|global|rate_limit"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "global rate_limit group must exist")
	// Both pods map to the same global key => single entry
	assert.Equal(t, 1, len(pg.entries))
}
