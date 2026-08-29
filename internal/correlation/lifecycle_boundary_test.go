package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestLifecycleDeadlinesTriggerAtExactTimestamp(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var actions []model.IncidentAction
	e := NewEngine(Config{
		Window:          10 * time.Minute,
		ResolveHoldDown: 5 * time.Minute,
		LifecycleHook: func(_ *model.Incident, action model.IncidentAction) {
			actions = append(actions, action)
		},
	})
	e.now = mockClock(now)

	ev := event.Event{Namespace: "default", PodName: "pod-1", Reason: "CrashLoopBackOff"}
	inc, action := e.Process(ev, "deploy-1", nil)
	require.Equal(t, model.ActionCreate, action)

	// Resolve hold-down also finalizes at its exact deadline.
	e.MarkResolved(inc.Key)
	now = now.Add(5 * time.Minute)
	e.now = mockClock(now)
	e.checkLifecycle()
	require.Equal(t, model.StateResolved, e.state[inc.Key].State)
	require.Contains(t, actions, model.ActionResolved)
}
