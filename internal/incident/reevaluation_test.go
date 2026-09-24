package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestReevaluateActiveEmitsOnlyActiveUnsuppressedIncidents(t *testing.T) {
	var actions []model.IncidentAction
	e := NewEngineWithClock(Config{
		Window: time.Minute,
		LifecycleHook: func(_ *model.Incident, action model.IncidentAction) {
			actions = append(actions, action)
		},
	}, clock.RealClock{})

	active, action := e.processEvent(event.Event{
		PodName: "api-1", Namespace: "apps", Reason: "Error",
	}, "api", nil)
	require.Equal(t, model.ActionCreate, action)
	require.NotNil(t, active)
	e.ReevaluateActive()
	require.Equal(t, []model.IncidentAction{
		model.ActionCreate, model.ActionReevaluate,
	}, actions)

	active.SuppressedBy = "node//worker"
	e.mu.Lock()
	e.state[active.Key].SuppressedBy = active.SuppressedBy
	e.mu.Unlock()
	actions = nil
	e.ReevaluateActive()
	require.Empty(t, actions)
}

func TestReevaluateActiveSkipsResolvedIncidents(t *testing.T) {
	var called bool
	e := NewEngineWithClock(Config{
		LifecycleHook: func(_ *model.Incident, _ model.IncidentAction) {
			called = true
		},
	}, clock.RealClock{})
	inc, _ := e.processEvent(event.Event{
		PodName: "api-1", Namespace: "apps", Reason: "Error",
	}, "api", nil)
	require.NotNil(t, inc)
	called = false
	e.mu.Lock()
	e.state[inc.Key].State = model.StateResolved
	e.mu.Unlock()
	e.ReevaluateActive()
	require.False(t, called)
}
