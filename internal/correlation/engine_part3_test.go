package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestEscalationSecondCrossingIsCritical(t *testing.T) {
	e := NewEngine(Config{
		Window:            10 * time.Minute,
		EscalationEnabled: true,
		EscalationTiers:   []int{1, 3, 5},
	})
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 0})
	e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 2},
	) // crosses tier 1 → high
	inc, action := e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 4},
	) // crosses tier 3 → critical
	assert.Equal(t, model.ActionUpdate, action)
	assert.Equal(t, model.SeverityCritical, inc.Severity)
}

func TestEscalationSameTierSkips(t *testing.T) {
	e := escTestEngine()
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	_, action := e.Process(
		ev,
		"dep",
		&model.ContainerState{RestartCount: 5},
	) // 4→5: no tier, same sig
	assert.Equal(t, model.ActionSkip, action)
}

func TestEscalationDisabledIsNoop(t *testing.T) {
	e := newTestEngine() // escalation off
	ev := event.Event{PodName: "p", Namespace: "ns", Reason: "OOMKilled"}
	e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	_, action := e.Process(ev, "dep", &model.ContainerState{RestartCount: 4})
	assert.Equal(t, model.ActionSkip, action) // no escalation, same sig
}

// BUG-2: inhibition tests.

func TestInhibitionSuppressesPodOnBrokenNode(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	nodeEv := event.Event{
		Resource: "node",
		PodName:  "node-1",
		NodeName: "node-1",
		Reason:   "NodeNotReady",
	}
	e.Process(nodeEv, "node-1", nil)
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(podEv, "dep", nil)
	assert.Nil(t, inc)
	assert.Equal(t, model.ActionSkip, action)
}

func TestInhibitionFlagOffDoesNotSuppress(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: false,
	})
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
	_, action := e.Process(
		event.Event{
			PodName:   "p",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"dep",
		nil,
	)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionOtherNodeUnaffected(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
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
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-2",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionLiftsOnNodeResolve(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
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
	e.ResolveByResource("node", "node-1")
	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionLiftsOnNodeResolveDuringHoldDown(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		ResolveHoldDown:           5 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
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
	assert.True(t, e.activeNodeIncidents["node-1"])

	// Node recovers: the incident enters PendingResolve (hold-down), but the
	// recovered node must stop suppressing pods immediately, not after the
	// hold-down finalizes.
	e.ResolveByResource("node", "node-1")
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods during hold-down",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionMarkResolvedHoldDownClearsFlag(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		ResolveHoldDown:           5 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
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
	assert.True(t, e.activeNodeIncidents["node-1"])

	// MarkResolved with hold-down enabled must clear the flag the same way
	// the immediate-resolve branch does.
	e.MarkResolved(BuildKey("", "node-1", "NodeNotReady", ""))
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods during hold-down",
	)
}

func TestInhibitionSuppressedCounter(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
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
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"dep",
		nil,
	)
	nodeInc := e.findNodeIncident("node-1")
	if nodeInc != nil {
		assert.Equal(t, 1, nodeInc.SuppressedPods)
		if assert.NotNil(t, nodeInc.SuppressedOwners) {
			assert.Equal(t, 1, nodeInc.SuppressedOwners["dep"])
		}
	}
}

func TestInhibitionOvercommitDoesNotSuppressPods(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Synthetic capacity incident — not a real node outage.
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeResourceCritical",
		},
		"node-1",
		nil,
	)
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"overcommit must not populate inhibition map",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"pod on over-committed node must not be suppressed",
	)
}

func TestInhibitionOvercommitDoesNotSuppressUnschedulable(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeResourceCritical",
		},
		"node-1",
		nil,
	)

	ev := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		NodeName:  "",
		Reason:    "Unschedulable",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(
		t,
		model.ActionCreate,
		action,
		"overcommit on one node must not suppress cluster-wide unschedulable "+
			"pods",
	)
}

func TestInhibitionRecoveredBaselineNodeClearsFlag(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Simulate a pre-existing NodeNotReady at startup (baselined, no incident).
	e.SetActiveNodeIncidents([]string{"node-1"})
	assert.True(t, e.activeNodeIncidents["node-1"])

	// Node recovers — no incident exists to resolve, but the flag must clear.
	e.RefreshNodeInhibition("node-1")
	assert.False(
		t,
		e.activeNodeIncidents["node-1"],
		"recovered node must stop suppressing pods",
	)

	podEv := event.Event{
		PodName:   "p",
		Namespace: "ns",
		NodeName:  "node-1",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(podEv, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
}

func TestInhibitionRefreshKeepsFlagWithOtherIncidents(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	// Two disruptive conditions on the same node.
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
	e.Process(
		event.Event{
			Resource: "node",
			PodName:  "node-1",
			NodeName: "node-1",
			Reason:   "NodeMemoryPressure",
		},
		"node-1",
		nil,
	)

	// One condition resolves — the other still active must keep the flag.
	e.MarkResolved(BuildKey("", "node-1", "NodeNotReady", ""))
	e.RefreshNodeInhibition("node-1")
	assert.True(
		t,
		e.activeNodeIncidents["node-1"],
		"remaining active node incident must keep suppression",
	)

	// Once the last active node incident resolves, the flag clears.
	e.MarkResolved(BuildKey("", "node-1", "NodeMemoryPressure", ""))
	e.RefreshNodeInhibition("node-1")
	assert.False(t, e.activeNodeIncidents["node-1"])
}
