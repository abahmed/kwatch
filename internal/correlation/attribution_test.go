package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// attributionEngine has every kind of cause armed at once: node-1 is down,
// node-2 sits under a mass failure, and Deployment "api" has a rollout
// incident of its own.
func attributionEngine(t *testing.T) *Engine {
	t.Helper()
	deadNodeMass := MassFailureKey("node//node-2")
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
		DependenciesOf: func(inc *model.Incident) []string {
			if inc.NodeName != "" {
				return []string{"node//" + inc.NodeName}
			}
			return nil
		},
	})
	e.now = mockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeNotReady",
		},
		"node-1",
		nil,
	)
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:    deadNodeMass,
			Reason: "ContainersNotReady",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	})
	_, action := e.Process(
		event.Event{
			Resource:  "deployment",
			Namespace: "ns",
			Reason:    "RolloutStuck",
		},
		OwnerPath("ns", "api"),
		nil,
	)
	require.Equal(t, model.ActionCreate, action)
	return e
}

// The broadest explanation wins: node, then shared dependency, then owner.
func TestAttributionPrecedence(t *testing.T) {
	e := attributionEngine(t)
	pod := func(node string) event.Event {
		return event.Event{
			PodName:   "api-" + node,
			Namespace: "ns",
			NodeName:  node,
			Reason:    "ContainersNotReady",
		}
	}

	e.mu.Lock()
	assert.Equal(
		t,
		causeNodeCondition,
		e.attribute(
			pod("node-1"),
			"api",
			BuildKey("ns", "api", "ContainersNotReady", ""),
			"pod",
		).kind,
		"a pod on a down node is the node's symptom even though its owner and "+
			"a mass failure would also claim it",
	)
	assert.Equal(
		t,
		causeSharedDependency,
		e.attribute(
			pod("node-2"),
			"api",
			BuildKey("ns", "api", "ContainersNotReady", ""),
			"pod",
		).kind,
		"off the down node, the mass failure claims it before the owner does",
	)
	assert.Equal(
		t,
		causeOwnerWorkload,
		e.attribute(
			pod("node-3"),
			"api",
			BuildKey("ns", "api", "ContainersNotReady", ""),
			"pod",
		).kind,
		"with no node or dependency to blame, the owning workload's incident "+
			"claims it",
	)
	assert.Equal(
		t,
		causeNone,
		e.attribute(
			pod("node-3"),
			"other",
			BuildKey("ns", "other", "ContainersNotReady", ""),
			"pod",
		).kind,
		"an unrelated owner on a healthy node is nobody's symptom",
	)
	assert.Equal(
		t,
		causeNone,
		e.attribute(
			event.Event{
				Resource: "node",
				NodeName: "node-2",
				Reason:   "DiskPressure",
			},
			"node-2",
			BuildKey("", "node-2", "DiskPressure", ""),
			"node",
		).kind,
		"a node is never a symptom of itself",
	)
	e.mu.Unlock()
}

// recordSymptom credits the cause and never announces the symptom.
func TestAttributionRecordsAgainstTheCause(t *testing.T) {
	e := attributionEngine(t)
	var announced int
	e.config.LifecycleHook = func(
		*model.Incident, model.IncidentAction,
	) {
		announced++
	}

	// Node: counted on the node incident, no incident of its own.
	inc, action := e.Process(
		event.Event{
			PodName:   "api-1",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "ContainersNotReady",
		},
		"api",
		nil,
	)
	assert.Equal(t, model.ActionSkip, action)
	assert.Nil(t, inc)
	e.mu.Lock()
	require.NotNil(t, e.findNodeIncident("node-1"))
	assert.Equal(t, 1, e.findNodeIncident("node-1").SuppressedPods)
	assert.Equal(t, 1, e.findNodeIncident("node-1").SuppressedOwners["api"])
	e.mu.Unlock()

	// Shared dependency: tracked, flagged, silent.
	inc, action = e.Process(
		event.Event{
			PodName:   "api-2",
			Namespace: "ns",
			NodeName:  "node-2",
			Reason:    "ContainersNotReady",
		},
		"api",
		nil,
	)
	assert.Equal(t, model.ActionSkip, action)
	require.NotNil(
		t,
		inc,
		"a mass-failure symptom is kept so it can be released later",
	)
	assert.Equal(t, MassFailureKey("node//node-2"), inc.SuppressedBy)

	// Owner: the workload incident absorbs the pod.
	inc, action = e.Process(
		event.Event{
			PodName:   "api-3",
			Namespace: "ns",
			NodeName:  "node-3",
			Reason:    "ContainersNotReady",
		},
		"api",
		nil,
	)
	assert.Equal(t, model.ActionSkip, action)
	assert.Nil(t, inc)
	e.mu.Lock()
	owner := e.state[BuildKey("ns", OwnerPath("ns", "api"), "RolloutStuck", "")]
	require.NotNil(t, owner)
	assert.Equal(t, 2, owner.Count)
	assert.True(t, owner.Resources["api-3"])
	e.mu.Unlock()

	assert.Zero(t, announced, "a symptom never speaks for itself")
}

// A key in post-resolve cooldown is still a symptom of its cause: attribution
// runs before the cooldown gate, so the owner incident keeps counting its pods
// instead of the event vanishing into "cooldown".
func TestAttributionOutranksCooldown(t *testing.T) {
	e := attributionEngine(t)
	podEv := event.Event{
		PodName:   "web-1",
		Namespace: "ns",
		NodeName:  "node-3",
		Reason:    "CrashLoopBackOff",
	}
	key := BuildKey("ns", "web", "CrashLoopBackOff", "")

	_, action := e.Process(podEv, "web", nil)
	require.Equal(t, model.ActionCreate, action)
	e.MarkResolved(key)
	e.mu.Lock()
	_, inCooldown := e.cleanupCooldown[key]
	e.mu.Unlock()
	require.True(t, inCooldown)

	// Now the owner gets its own incident and the pod fails again.
	_, action = e.Process(
		event.Event{
			Resource:  "deployment",
			Namespace: "ns",
			Reason:    "RolloutStuck",
		},
		OwnerPath("ns", "web"),
		nil,
	)
	require.Equal(t, model.ActionCreate, action)
	inc, action := e.Process(podEv, "web", nil)
	assert.Equal(t, model.ActionSkip, action)
	assert.Nil(t, inc)
	e.mu.Lock()
	owner := e.state[BuildKey("ns", OwnerPath("ns", "web"), "RolloutStuck", "")]
	assert.Equal(
		t,
		2,
		owner.Count,
		"the cooling-down pod is still credited to its owner",
	)
	e.mu.Unlock()
}

// A recurrence during the resolve hold-down must revive the incident, not be
// swallowed. ResolveByResource used to arm the cooldown at hold-down time,
// which made exactly that happen; every resolve path now shares one helper.
func TestHoldDownDoesNotArmCooldown(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(
		Config{Window: 10 * time.Minute, ResolveHoldDown: 5 * time.Minute},
	)
	e.now = mockClock(now)
	ev := event.Event{
		Resource: "node",
		PodName:  "node-1",
		NodeName: "node-1",
		Reason:   "NodeNotReady",
	}
	_, action := e.Process(ev, "node-1", nil)
	require.Equal(t, model.ActionCreate, action)
	key := BuildKey("", "node-1", "NodeNotReady", "")

	e.ResolveByResource("node", "node-1")
	e.mu.Lock()
	assert.Equal(t, model.StatePendingResolve, e.state[key].State)
	_, armed := e.cleanupCooldown[key]
	e.mu.Unlock()
	assert.False(t, armed, "hold-down must not arm the cooldown")

	// The condition comes back inside the hold-down: the incident revives.
	inc, action := e.Process(ev, "node-1", nil)
	assert.Equal(
		t,
		model.ActionSkip,
		action,
		"revival is silent — nothing observable changed",
	)
	require.NotNil(t, inc, "the event must not be swallowed by a cooldown")
	assert.Equal(t, model.StateActive, inc.State)
}
