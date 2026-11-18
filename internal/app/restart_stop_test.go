package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fakeAttributes is a model of a few attributes.
type fakeAttributes map[inventory.EntityID]map[string]inventory.Value

func (f fakeAttributes) Attribute(
	id inventory.EntityID, name string,
) (inventory.Attribute, bool) {
	v, ok := f[id][name]
	return inventory.Attribute{Value: v}, ok
}

func nodeID(name string) inventory.EntityID {
	return inventory.CoreID(kube.KindNode, "", name)
}

func cordonedNode(name string) fakeAttributes {
	return fakeAttributes{nodeID(name): {
		kube.AttrUnschedulable: inventory.Bool(true),
		kube.AttrReady:         inventory.Bool(true),
		kube.AttrTaints: inventory.Text(
			"node.kubernetes.io/unschedulable:NoSchedule"),
	}}
}

// A drained node stops kwatch with SIGTERM, a clean stop: the next
// start must say the node was drained, not "normal restart".
func TestStartupReportsDrainedNodeAfterCleanStop(t *testing.T) {
	t.Setenv("POD_NAME", "kwatch-5d8f-abcde")
	t.Setenv("NODE_NAME", "worker-a")
	disk := diskState{store: openTestStore(t)}
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

	first := startAt(t, disk, t0)
	first.noteStop(cordonedNode("worker-a"))
	first.EndSession(context.Background(), "graceful_shutdown")

	// Same node name: only the recorded facts tell the drain.
	t.Setenv("POD_NAME", "kwatch-5d8f-fghij")
	second := startAt(t, disk, t0.Add(time.Minute))
	require.NotNil(t, second.lastStop)
	assert.Equal(t, "node_disruption", second.lastStop.Reason)
	assert.Equal(t, "its node was drained, shut down or removed",
		second.lastStop.Text)
	assert.Equal(t, "node_disruption", second.gap().restart.Reason)
}

// Same template, another node, no recorded facts (an older kwatch
// wrote the stop): the pod was moved off its node.
func TestStartupReportsMovedPodAsNodeDisruption(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	t.Setenv("POD_NAME", "kwatch-5d8f-fghij")
	t.Setenv("NODE_NAME", "worker-b")
	store := &sessionStore{
		testStateStore: testStateStore{version: "dev", clusterID: "c"},
		session: runtimeSession{SessionID: "old",
			PodName: "kwatch-5d8f-abcde", NodeName: "worker-a",
			EndedAt: now.Add(-time.Minute), EndReason: "graceful_shutdown"},
	}
	sm := newStartupManager(store, true,
		clock.Func(func() time.Time { return now }))
	_, err := sm.Start(context.Background())
	require.NoError(t, err)
	require.NotNil(t, sm.lastStop)
	assert.Equal(t, "node_disruption", sm.lastStop.Reason)
}

func TestClassifyStopExplainsAStopOnSignal(t *testing.T) {
	here := runIdentity{Pod: "kwatch-5d8f-fghij", Node: "worker-a",
		Version: "v2", Config: "c1"}
	base := runtimeSession{SessionID: "s", PodName: "kwatch-5d8f-abcde",
		NodeName: "worker-a", Version: "v2", Config: "c1",
		EndedAt: time.Now(), EndReason: "graceful_shutdown"}
	cases := []struct {
		name string
		edit func(*runtimeSession)
		want string
	}{
		{"plain restart", func(*runtimeSession) {}, ""},
		{"cordoned", func(s *runtimeSession) {
			s.Stop = &stopFacts{Signal: true, NodeCordoned: true}
		}, "node_disruption"},
		{"termination taint", func(s *runtimeSession) {
			s.Stop = &stopFacts{NodeTaint: "karpenter.sh/disrupted"}
		}, "node_disruption"},
		{"node not ready", func(s *runtimeSession) {
			s.Stop = &stopFacts{NodeNotReady: true}
		}, "node_disruption"},
		{"evicted pod", func(s *runtimeSession) {
			s.Stop = &stopFacts{PodDisruption: "EvictionByEvictionAPI"}
		}, "eviction"},
		{"taint manager", func(s *runtimeSession) {
			s.Stop = &stopFacts{PodDisruption: "DeletionByTaintManager"}
		}, "node_disruption"},
		{"new version", func(s *runtimeSession) { s.Version = "v1" },
			"deployment_rollout"},
		{"new config", func(s *runtimeSession) { s.Config = "c0" },
			"deployment_rollout"},
		{"new template", func(s *runtimeSession) {
			s.PodName, s.NodeName = "kwatch-77aa-abcde", "worker-b"
		}, "deployment_rollout"},
		{"moved", func(s *runtimeSession) { s.NodeName = "worker-b" },
			"node_disruption"},
		{"signal only", func(s *runtimeSession) {
			s.Stop = &stopFacts{Signal: true}
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous := base
			tc.edit(&previous)
			assert.Equal(t, tc.want, classifyRestartAt(previous, here))
		})
	}
}

func TestReadStopFactsReadsOwnNodeAndPod(t *testing.T) {
	pod := inventory.CoreID(kube.KindPod, "kwatch", "kwatch-1")
	key := kube.ConditionKey("DisruptionTarget")
	model := fakeAttributes{
		nodeID("n1"): {
			kube.AttrUnschedulable: inventory.Bool(false),
			kube.AttrDeleting:      inventory.Bool(true),
			kube.AttrReady:         inventory.Bool(false),
			kube.AttrTaints: inventory.Text("dedicated=infra:NoSchedule," +
				"aws-node-termination-handler/spot-itn:NoSchedule"),
		},
		pod: {
			key:                            inventory.Text("True"),
			key + kube.AttrConditionReason: inventory.Text("PreemptionByScheduler"),
		},
	}
	got := readStopFacts(model, "kwatch", "kwatch-1", "n1")
	assert.Equal(t, stopFacts{NodeDeleting: true, NodeNotReady: true,
		NodeTaint:     "aws-node-termination-handler/spot-itn",
		PodDisruption: "PreemptionByScheduler"}, got)
	assert.Equal(t, stopFacts{},
		readStopFacts(model, "kwatch", "other", "n2"), "not in the model")
	assert.Equal(t, stopFacts{}, readStopFacts(nil, "kwatch", "p", "n"))
}

func TestDisruptionTaintIgnoresOrdinaryTaints(t *testing.T) {
	assert.Equal(t, "", disruptionTaint(""))
	assert.Equal(t, "", disruptionTaint("dedicated=infra:NoSchedule"))
	assert.Equal(t, "", disruptionTaint(
		"DeletionCandidateOfClusterAutoscaler=1:PreferNoSchedule"))
	assert.Equal(t, "ToBeDeletedByClusterAutoscaler", disruptionTaint(
		"ToBeDeletedByClusterAutoscaler=1:NoSchedule"))
	assert.Equal(t, "cloud.google.com/impending-node-termination",
		disruptionTaint("cloud.google.com/impending-node-termination:NoSchedule"))
}

// Facts from an old pipeline stop do not describe the session end.
func TestFactsAtStopDropsOldFacts(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	sm := newStartupManager(&testStateStore{}, true,
		clock.Func(func() time.Time { return now }))
	sm.session.NodeName = "n1"
	sm.noteStop(cordonedNode("n1"))
	assert.Equal(t, &stopFacts{Signal: true, NodeCordoned: true,
		NodeTaint: "node.kubernetes.io/unschedulable"}, sm.factsAtStop(now))
	assert.Equal(t, &stopFacts{Signal: true},
		sm.factsAtStop(now.Add(stopFactsMaxAge+time.Second)))
}

// An evicted pod stays behind with reason Evicted; a clean stop of a
// replaced pod asks the API, a restart in place does not.
func TestStopOnSignalReadsEvictionEvidence(t *testing.T) {
	previous := runtimeSession{SessionID: "s", PodName: "kwatch-5d8f-a",
		NodeName: "n1", EndedAt: time.Now(),
		EndReason: "graceful_shutdown"}
	here := runIdentity{Pod: "kwatch-5d8f-b", Node: "n1"}
	require.True(t, wantsEvidence(previous, here))
	assert.False(t, wantsEvidence(previous,
		runIdentity{Pod: "kwatch-5d8f-a", Node: "n1"}))
	assert.Equal(t, "eviction", classifyWithEvidence(previous,
		restartEvidence{PodReason: "Evicted", NodeReady: true}, ""))
	assert.Equal(t, "node_disruption", classifyWithEvidence(previous,
		restartEvidence{NodeObserved: true, NodeReason: "NotFound"}, ""))
	assert.Equal(t, "", classifyWithEvidence(previous,
		restartEvidence{NodeObserved: true, NodeReady: true,
			LeaseLost: true}, ""), "a lease change is not a reason")
	assert.Equal(t, "deployment_rollout", classifyWithEvidence(previous,
		restartEvidence{PodReason: "Evicted"}, "deployment_rollout"))
}
