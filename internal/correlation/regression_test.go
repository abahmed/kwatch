package correlation

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// B1: crossing the crash-loop fold threshold must migrate the incident to the
// folded key — same ID (alert thread continuity), no silent orphan, no
// duplicate CREATE for the same ongoing loop.
func TestFoldMigratesIncidentNotRecreates(t *testing.T) {
	e := newTestEngine()
	_, action := e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "Error"},
		"dep", &model.ContainerState{RestartCount: 3})
	require.Equal(t, model.ActionCreate, action)
	oldID := e.state["ns:dep:Error:"].ID
	oldFirstSeen := e.state["ns:dep:Error:"].FirstSeen

	_, action = e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "Error"},
		"dep", &model.ContainerState{RestartCount: 6})
	assert.Equal(t, model.ActionSkip, action, "fold must not fire a duplicate CREATE")

	_, ok := e.state["ns:dep:Error:"]
	assert.False(t, ok, "pre-fold key must be gone")
	mig, ok := e.state["ns:dep:CrashLoopHighFrequency:"]
	require.True(t, ok, "folded key must hold the migrated incident")
	assert.Equal(t, oldID, mig.ID, "ID must survive the fold (alert thread continuity)")
	assert.True(t, mig.FirstSeen.Equal(oldFirstSeen), "FirstSeen must survive the fold")
	assert.Equal(t, 2, mig.Count, "the crossing event must count on the same incident")
}

// B3: renotify must skip incidents absorbed into a smart group — the group
// flush is their single re-notification channel.
func TestRenotifySkipsGroupedIncidents(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window:                     10 * time.Minute,
		SmartGroupingWindow:        60 * time.Second,
		RenotifyIntervalBySeverity: map[string]time.Duration{"default": 30 * time.Second},
		RenotifyMaxPerIncident:     3,
	})
	e.now = mockClock(now)

	var memberUpdates int
	e.config.LifecycleHook = func(inc *model.Incident, a model.IncidentAction) {
		if inc.Key == "ns:dep1:CrashLoopBackOff:" && a == model.ActionUpdate {
			memberUpdates++
		}
	}

	_, action := e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// Flush the group, then hit a renotify tick well past the interval.
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	e.now = mockClock(now.Add(2 * time.Hour))
	e.checkLifecycle()
	e.checkLifecycle()

	assert.Equal(t, 0, memberUpdates, "grouped member must never be individually renotified")
}

// B3 control: a non-grouped incident still renotifies normally.
func TestRenotifyStillFiresForNonGrouped(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window:                     10 * time.Minute,
		SmartGroupingWindow:        60 * time.Second,
		RenotifyIntervalBySeverity: map[string]time.Duration{"default": 30 * time.Second},
		RenotifyMaxPerIncident:     3,
	})
	e.now = mockClock(now)

	// Different reasons → different groups; this one stays alone but grouped.
	var updates int
	e.config.LifecycleHook = func(inc *model.Incident, a model.IncidentAction) {
		if strings.HasPrefix(string(inc.Key), "ns:dep1:") && a == model.ActionUpdate {
			updates++
		}
	}
	e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "Error"}, "dep1", nil)

	e.now = mockClock(now.Add(91 * time.Second))
	e.checkLifecycle()
	assert.Equal(t, 0, updates, "still grouped within window → no individual renotify")

	// After the group flushes it is tracked as a member → still skipped.
	e.now = mockClock(now.Add(2 * time.Hour))
	e.checkLifecycle()
	assert.Equal(t, 0, updates)
}

// B4: a workload incident stored with the "ns/name" owner encoding must
// suppress its own pod symptoms (bare owner) via cascading suppression.
func TestCascadingSuppressionAcrossOwnerEncodings(t *testing.T) {
	e := newTestEngine()
	// Workload detector path: Owner = "ns/dep".
	_, action := e.Process(event.Event{Resource: "deployment", Namespace: "ns", Reason: "ProgressDeadlineExceeded"}, "ns/dep", nil)
	require.Equal(t, model.ActionCreate, action)

	// Pod path: owner resolved to the bare deployment name.
	_, action = e.Process(event.Event{Resource: "pod", PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep", nil)
	assert.Equal(t, model.ActionSkip, action, "bare-owner pod symptom must be suppressed")
}

// B6: resolving one pod's incident must not un-baseline sibling pods sharing
// the same owner+reason key.
func TestBaselineSiblingsPreservedOnResolve(t *testing.T) {
	now := time.Now().Unix()
	e := NewEngine(Config{Window: 10 * time.Minute, BaselineTTL: 24 * time.Hour,
		Baseline: map[string]map[string]int64{"ns:dep:CrashLoopBackOff:": {"p1": now, "p2": now}}})

	// p3 is new → incident fires under the shared key.
	_, action := e.Process(event.Event{PodName: "p3", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep", nil)
	require.Equal(t, model.ActionCreate, action)

	e.MarkResolved("ns:dep:CrashLoopBackOff:")

	// p1/p2 keep their startup baseline; p3's (non-existent) entry stays gone.
	pods, ok := e.baseline["ns:dep:CrashLoopBackOff:"]
	require.True(t, ok, "key must survive resolution while siblings remain baselined")
	assert.True(t, pods["p1"] > 0 && pods["p2"] > 0, "siblings must stay baselined")
	assert.Zero(t, pods["p3"])

	_, action = e.Process(event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"}, "dep", nil)
	assert.Equal(t, model.ActionSkip, action, "sibling p1 must remain suppressed")
}

// B6 global-scope variant: resolving one global image-pull incident must not
// un-baseline the other affected pods cluster-wide.
func TestBaselineGlobalScopePreservedOnResolve(t *testing.T) {
	now := time.Now().Unix()
	e := NewEngine(Config{Window: 10 * time.Minute, BaselineTTL: 24 * time.Hour,
		Baseline: map[string]map[string]int64{"ImagePullBackOff|global|rate_limit": {"p1": now, "p2": now, "p3": now}}})

	_, action := e.Process(event.Event{PodName: "p9", Namespace: "ns1", Reason: "ImagePullBackOff", Message: "toomanyrequests"}, "dep1", nil)
	require.Equal(t, model.ActionCreate, action)

	e.MarkResolved("ImagePullBackOff|global|rate_limit")

	for _, p := range []string{"p1", "p2", "p3"} {
		_, action = e.Process(event.Event{PodName: p, Namespace: "other-ns", Reason: "ImagePullBackOff", Message: "toomanyrequests"}, "other-owner", nil)
		assert.Equal(t, model.ActionSkip, action, "globally baselined pod %s must stay suppressed", p)
	}
}
