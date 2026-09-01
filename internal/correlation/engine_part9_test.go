package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMassFailureSuppressesItsMembers(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deadNode := "node//ip-10-0-81-7"

	e := NewEngine(Config{
		Window: 10 * time.Minute,
		DependenciesOf: func(inc *model.Incident) []string {
			if inc.NodeName != "" {
				return []string{"node//" + inc.NodeName}
			}
			return nil
		},
	})
	e.now = mockClock(now)

	// Without a tracked mass failure, each workload alerts on its own.
	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		_, action := e.Process(event.Event{
			PodName: dep + "-abc", Namespace: "ns",
			Reason: "ContainersNotReady", NodeName: "ip-10-0-81-7",
		}, dep, nil)
		assert.NotEqual(
			t,
			model.ActionSkip,
			action,
			"%s should alert while nothing explains it",
			dep,
		)
	}

	// The detector now attributes the failures to one dead node.
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:    MassFailureKey(deadNode),
			Reason: "ContainersNotReady",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	},
	)

	// Further workloads on that node are symptoms of an alert already sent.
	for _, dep := range []string{"dep4", "dep5", "dep6"} {
		_, action := e.Process(event.Event{
			PodName: dep + "-abc", Namespace: "ns",
			Reason: "ContainersNotReady", NodeName: "ip-10-0-81-7",
		}, dep, nil)
		assert.Equal(
			t,
			model.ActionSkip,
			action,
			"%s is covered by the mass-failure alert",
			dep,
		)
	}

	// A workload on a healthy node is unrelated and must still alert.
	_, action := e.Process(event.Event{
		PodName: "dep7-abc", Namespace: "ns",
		Reason: "ContainersNotReady", NodeName: "ip-10-0-99-9",
	}, "dep7", nil)
	assert.NotEqual(
		t,
		model.ActionSkip,
		action,
		"a different node is not covered",
	)

	// The node's own incident is the root cause, never a symptom of itself.
	_, action = e.Process(event.Event{
		Resource: "node", PodName: "ip-10-0-81-7", Namespace: "",
		Reason: "NodeNotReady", NodeName: "ip-10-0-81-7",
	}, "ip-10-0-81-7", nil)
	assert.NotEqual(
		t,
		model.ActionSkip,
		action,
		"node incidents must never be suppressed",
	)
}

// A lone owner is not a group. It is announced at once, under its own key, and
// the grouping window emits nothing for it later.
func TestSmartGroupingSingleMemberEmitsIncidentNotGroup(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	inc, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"no waiting for a grouping window",
	)
	assert.Equal(t, model.IncidentKey("ns:dep1:CrashLoopBackOff:"), inc.Key)
	assert.False(t, IsGroupKey(inc.Key))

	var emitted int
	e.config.LifecycleHook = func(
		*model.Incident, model.IncidentAction,
	) {
		emitted++
	}
	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()
	assert.Equal(
		t,
		0,
		emitted,
		"nothing left to emit for an already-announced lone owner",
	)

	// Two members that share a real dimension still become a group.
	sigLog := "connection refused:5432"
	e2 := newSmartGroupingEngine()
	e2.now = mockClock(now)
	e2.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep1",
		nil,
	)
	e2.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep2",
		nil,
	)
	var grouped bool
	e2.config.LifecycleHook = func(
		inc *model.Incident, _ model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			grouped = true
		}
	}
	e2.now = mockClock(now.Add(61 * time.Second))
	e2.checkLifecycle()
	assert.True(t, grouped, "two members must still be announced as a group")
}

func TestSmartGroupingFlushAfterWindow(t *testing.T) {
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

	var groupInc *model.Incident
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupInc = inc
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.NotNil(t, groupInc, "group summary must be emitted")
	assert.Equal(t, "CrashLoopBackOff", groupInc.Reason)
	assert.Equal(t, 2, groupInc.Count)
	assert.Contains(t, groupInc.Hint, "dep1")
	assert.Contains(t, groupInc.Hint, "dep2")
}

func TestSmartGroupingDifferentReasonsSeparate(t *testing.T) {
	e := newSmartGroupingEngine()
	e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	e.Process(
		event.Event{PodName: "p2", Namespace: "ns", Reason: "OOMKilled"},
		"dep1",
		nil,
	)

	var groups int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groups++
		}
	}
	e.checkLifecycle()
	assert.Equal(t, 0, groups)
}

func TestSmartGroupingResolvedNotIncluded(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newSmartGroupingEngine()
	e.now = mockClock(now)

	// Three members share a signature, so removing one still leaves a real
	// group of two rather than a lone incident.
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
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			Reason:    "CrashLoopBackOff",
			Logs:      sigLog,
		},
		"dep3",
		nil,
	)

	e.MarkResolved("ns:dep1:CrashLoopBackOff:")

	var groupCount int
	e.config.LifecycleHook = func(
		inc *model.Incident, action model.IncidentAction,
	) {
		if IsGroupKey(inc.Key) {
			groupCount++
		}
	}

	e.now = mockClock(now.Add(61 * time.Second))
	e.checkLifecycle()

	require.Equal(t, 1, groupCount)
}

func TestBuildGroupSummary(t *testing.T) {
	e := newSmartGroupingEngine()
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e.now = mockClock(now)
	entries := []groupEntry{
		{
			namespace: "ns",
			owner:     "dep1",
			reason:    "CrashLoopBackOff",
			podName:   "p1",
		},
		{
			namespace: "ns",
			owner:     "dep2",
			reason:    "CrashLoopBackOff",
			podName:   "p2",
		},
		{
			namespace: "ns",
			owner:     "dep1",
			reason:    "CrashLoopBackOff",
			podName:   "p3",
		},
	}
	summary := e.buildGroupSummary(entries)
	// The reason, the count and the age are rendered as their own fields; the
	// summary names what is affected and nothing else.
	assert.NotContains(
		t,
		summary,
		"CrashLoopBackOff",
		"reason is shown separately",
	)
	assert.NotContains(t, summary, "total", "count is shown separately")
	assert.Contains(t, summary, "2 workloads in ns")
	assert.Contains(
		t,
		summary,
		"dep1 ×2",
		"an owner with several failing pods says so",
	)
	assert.Contains(t, summary, "dep2")
	_ = now
}

func TestBuildGroupSummaryEmpty(t *testing.T) {
	e := newSmartGroupingEngine()
	assert.Equal(t, "", e.buildGroupSummary(nil))
	assert.Equal(t, "", e.buildGroupSummary([]groupEntry{}))
}

func TestSmartGroupingWindowConfigZeroDisabled(t *testing.T) {
	e := NewEngine(Config{
		Window:              10 * time.Minute,
		SmartGroupingWindow: 0,
	})
	_, action := e.Process(
		event.Event{PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff"},
		"dep1",
		nil,
	)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"window=0 should disable buffering",
	)
}
