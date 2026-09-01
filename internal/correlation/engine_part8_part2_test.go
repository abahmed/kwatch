package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMassFailureSuppressionReleasesSurvivorsWhenItClears(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	deadNode := MassFailureKey("node//ip-10-0-81-7")
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
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:    deadNode,
			Reason: "ContainersNotReady",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	})

	type seen struct {
		name   string
		action model.IncidentAction
	}
	var announced []seen
	e.config.LifecycleHook = func(inc *model.Incident, a model.IncidentAction) {
		if a != model.ActionSkip {
			announced = append(announced, seen{inc.Name, a})
		}
	}

	ev := func(dep string) event.Event {
		return event.Event{
			PodName:   dep + "-abc",
			Namespace: "ns",
			Reason:    "ContainersNotReady",
			NodeName:  "ip-10-0-81-7",
		}
	}
	for _, dep := range []string{"dep1", "dep2", "dep3"} {
		inc, action := e.Process(ev(dep), dep, nil)
		assert.Equal(t, model.ActionSkip, action)
		require.NotNil(t, inc, "a suppressed incident is still recorded")
		assert.Equal(t, deadNode, inc.SuppressedBy)
	}
	assert.Equal(
		t,
		3,
		e.ActiveCount(),
		"suppressed incidents count toward the mass failure",
	)
	assert.Empty(
		t,
		announced,
		"nothing under a mass failure is announced on its own",
	)

	// One member recovers while suppressed: no notification for something never
	// announced.
	e.MarkResolved(BuildKey("ns", "dep1", "ContainersNotReady", ""))
	assert.Empty(t, announced, "a suppressed incident resolves silently")

	// The mass failure clears; the two survivors are released and announced
	// by the engine itself — no caller has to remember to do it.
	released := e.ReleaseSuppressed(deadNode)
	require.Equal(t, 2, released, "only still-active members are released")
	require.Len(t, announced, 2)
	for _, a := range announced {
		assert.Equal(t, model.ActionCreate, a.action)
		assert.NotEqual(
			t,
			"dep1",
			a.name,
			"the resolved member is not re-announced",
		)
	}
	// From here on they behave like any other incident: they were announced
	// on release, so their recovery is announced too.
	e.MarkResolved(BuildKey("ns", "dep2", "ContainersNotReady", ""))
	assert.Equal(
		t,
		seen{"dep2", model.ActionResolved},
		announced[len(announced)-1],
		"a released incident's recovery is visible",
	)
}
