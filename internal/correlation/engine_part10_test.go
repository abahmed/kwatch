package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSmartGroupingPendingGroupCleanedAfterFlush(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	e.mu.Lock()
	pg, exists := e.groupBuffers["CrashLoopBackOff|ns|dep1"]
	e.mu.Unlock()
	assert.False(t, exists, "pending group must be deleted after flush")
	require.Nil(t, pg)
}

func TestSmartGroupingIncidentHasNotifiedSig(t *testing.T) {
	e := newSmartGroupingEngine()
	// First owner: announced immediately, so its signature is the real one.
	inc, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(t, model.ActionCreate, action)
	require.NotNil(t, inc)
	assert.NotZero(t, inc.NotifiedSig, "NotifiedSig must be set")
	assert.NotZero(t, inc.LastNotifiedAt, "LastNotifiedAt must be set")
	// Second owner: buffered, and the signature is set to hold it back.
	inc2, action := e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep2",
		nil,
	)
	assert.Equal(t, model.ActionSkip, action)
	require.NotNil(t, inc2)
	assert.NotZero(
		t,
		inc2.NotifiedSig,
		"a buffered incident carries a signature so it is not re-buffered",
	)
}

func TestSmartGroupingReFlushUpdateNotCreate(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	// Two owners sharing a log signature form one genuine group. A buffer
	// holding a single member is emitted as that member, not as a group.
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

	var groupInc *model.Incident
	var groupAction model.IncidentAction
	var groupActions int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupInc = inc
			groupAction = action
			groupActions++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, 1, groupActions)
	require.Equal(t, model.ActionCreate, groupAction)
	key := groupInc.Key

	// Re-arm the buffer with more events on the same group.
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

	// Flush again past the renotify cooldown: same key, UPDATE not CREATE.
	e.now = mockClock(now.Add(7 * time.Minute))
	e.checkLifecycle()

	require.Equal(t, 2, groupActions)
	assert.Equal(
		t,
		key,
		groupInc.Key,
		"re-flush must keep the stable group key",
	)
	assert.Equal(
		t,
		model.ActionUpdate,
		groupAction,
		"re-flush must emit an update, not a create",
	)
}

func TestSmartGroupingReFlushCooldownSuppresses(t *testing.T) {
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
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupCalls int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCalls++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	assert.Equal(t, 1, groupCalls)

	// Re-arm the buffer and flush within the cooldown: no re-notification.
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(122 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		1,
		groupCalls,
		"re-flush within the cooldown must not re-notify",
	)
}

func TestSmartGroupingReGroupAfterCooldownSkip(t *testing.T) {
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
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)

	var groupCalls int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCalls++
		}
	}

	// First flush emits CREATE and resets the member's NotifiedSig.
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	require.Equal(t, 1, groupCalls)

	// Re-arm with the same member and flush within the renotify cooldown:
	// suppressed, but the member must remain re-groupable afterward.
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
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(122 * time.Second))
	e.checkLifecycle()
	require.Equal(
		t,
		1,
		groupCalls,
		"re-flush within the cooldown must not re-notify",
	)

	// The suppressed flush must reset NotifiedSig so the member can re-enter
	// the buffer; once the cooldown lapses the recurring flush must emit an
	// UPDATE rather than staying silent forever.
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
			PodName:   "p1b",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	e.now = mockClock(now.Add(400 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		2,
		groupCalls,
		"group must resume UPDATE notifications after the cooldown lapses",
	)
}
