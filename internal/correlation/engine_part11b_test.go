package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSmartGroupingFoldRekeysMember(t *testing.T) {
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

	// A high-frequency member migrates to its folded incident key.
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

	_, ok := e.state["ns:dep1:CrashLoopHighFrequency:"]
	require.True(t, ok, "folded incident must migrate to the folded key")
	_, ok = e.state["ns:dep1:CrashLoopBackOff:"]
	require.False(t, ok, "old key must be gone")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate},
		actions,
		"fold must not resolve or re-notify the group",
	)

	// Resolving only dep2 leaves the folded dep1 member active.
	e.MarkResolved("ns:dep2:CrashLoopBackOff:")
	require.Equal(t, []model.IncidentAction{model.ActionCreate}, actions)

	e.MarkResolved("ns:dep1:CrashLoopHighFrequency:")
	require.Equal(
		t,
		[]model.IncidentAction{model.ActionCreate, model.ActionResolved},
		actions,
	)
}
