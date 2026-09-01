package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestCleanupCooldownExpires(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	// Create incident
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	_, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)

	// Advance past Window + 1s so cooldown expires
	fakeNow = fakeNow.Add(11*time.Minute + 1*time.Second)
	e.now = mockClock(fakeNow)

	// Cleanup
	e.cleanup()

	// Advance past Window again (cooldown = Window = 10 min)
	fakeNow = fakeNow.Add(11 * time.Minute)
	e.now = mockClock(fakeNow)

	// Same event — should create new incident (cooldown expired)
	inc, action := e.Process(ev, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
}

func TestSuppressedOwnersTracked(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})

	// Create node incident and populate inhibition
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

	// Suppress pods from different owners on the same node
	e.Process(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"deploy-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p2",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "OOMKilled",
		},
		"deploy-1",
		nil,
	)
	e.Process(
		event.Event{
			PodName:   "p3",
			Namespace: "ns",
			NodeName:  "node-1",
			Reason:    "CrashLoopBackOff",
		},
		"statefulset-1",
		nil,
	)

	// Verify SuppressedOwners on the node incident
	nodeInc := e.findNodeIncident("node-1")
	if assert.NotNil(t, nodeInc) {
		assert.Equal(t, 3, nodeInc.SuppressedPods)
		assert.Equal(t, 2, nodeInc.SuppressedOwners["deploy-1"])
		assert.Equal(t, 1, nodeInc.SuppressedOwners["statefulset-1"])
	}
}

func TestUnschedulableSuppressedDuringNodeIncident(t *testing.T) {
	e := NewEngine(Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})

	// Create node incident
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

	// Unschedulable pod (empty NodeName) — should be suppressed
	ev := event.Event{
		PodName:   "p1",
		Namespace: "ns",
		NodeName:  "",
		Reason:    "Unschedulable",
	}
	_, action := e.Process(ev, "deploy-1", nil)
	assert.Equal(t, model.ActionSkip, action)

	// Verify SuppressedPods incremented on the node incident
	nodeInc := e.findNodeIncident("node-1")
	if assert.NotNil(t, nodeInc) {
		assert.Equal(t, 1, nodeInc.SuppressedPods)
		assert.Equal(t, 1, nodeInc.SuppressedOwners["deploy-1"])
	}
}

// Mock listers used by isOwnerHealthy tests.

type mockDeployLister struct {
	appsv1lister.DeploymentLister
	getFn func(ns, name string) (*appsv1.Deployment, error)
}

func (m *mockDeployLister) Deployments(
	namespace string,
) appsv1lister.DeploymentNamespaceLister {
	return &mockDeployNsLister{
		getFn: func(name string) (*appsv1.Deployment, error) {
			return m.getFn(namespace, name)
		},
	}
}

type mockDeployNsLister struct {
	appsv1lister.DeploymentNamespaceLister
	getFn func(name string) (*appsv1.Deployment, error)
}

func (m *mockDeployNsLister) Get(name string) (*appsv1.Deployment, error) {
	return m.getFn(name)
}

func (m *mockDeployNsLister) List(
	selector labels.Selector,
) ([]*appsv1.Deployment, error) {
	return nil, nil
}

type mockSSLister struct {
	appsv1lister.StatefulSetLister
	getFn func(ns, name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSLister) StatefulSets(
	namespace string,
) appsv1lister.StatefulSetNamespaceLister {
	return &mockSSNsLister{
		getFn: func(name string) (*appsv1.StatefulSet, error) {
			return m.getFn(namespace, name)
		},
	}
}

type mockSSNsLister struct {
	appsv1lister.StatefulSetNamespaceLister
	getFn func(name string) (*appsv1.StatefulSet, error)
}

func (m *mockSSNsLister) Get(name string) (*appsv1.StatefulSet, error) {
	return m.getFn(name)
}

func (m *mockSSNsLister) List(
	selector labels.Selector,
) ([]*appsv1.StatefulSet, error) {
	return nil, nil
}

type mockDSLister struct {
	appsv1lister.DaemonSetLister
	getFn func(ns, name string) (*appsv1.DaemonSet, error)
}

func (m *mockDSLister) DaemonSets(
	namespace string,
) appsv1lister.DaemonSetNamespaceLister {
	return &mockDSNsLister{getFn: func(name string) (*appsv1.DaemonSet, error) {
		return m.getFn(namespace, name)
	}}
}

type mockDSNsLister struct {
	appsv1lister.DaemonSetNamespaceLister
	getFn func(name string) (*appsv1.DaemonSet, error)
}

func (m *mockDSNsLister) Get(name string) (*appsv1.DaemonSet, error) {
	return m.getFn(name)
}

func (m *mockDSNsLister) List(
	selector labels.Selector,
) ([]*appsv1.DaemonSet, error) {
	return nil, nil
}

func TestSnapshotAllRestoreIncidentsRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	require.NotNil(t, inc)
	require.NotEmpty(t, inc.Key)

	// Snapshot
	snap := e.SnapshotAll()
	require.NotNil(t, snap)
	require.Contains(t, snap, inc.Key)

	snapped := snap[inc.Key]
	assert.Equal(t, inc.Reason, snapped.Reason)
	assert.Equal(t, inc.Count, snapped.Count)
	assert.Equal(t, inc.State, snapped.State)

	// Restore into a fresh engine with matching baseline
	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	e2.RestoreIncidents(snap)
	assert.Equal(t, 1, e2.ActiveCount())

	// Verify restored incidents are accessible
	e2.mu.Lock()
	restored, exists := e2.state[inc.Key]
	e2.mu.Unlock()
	assert.True(t, exists)
	assert.Equal(t, inc.Reason, restored.Reason)
	assert.Equal(t, inc.Namespace, restored.Namespace)
	assert.False(t, restored.LastSeen.IsZero())
}

func TestSnapshotPersistedRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 2})
	require.NotNil(t, inc)

	snap := e.SnapshotPersisted()
	require.NotNil(t, snap)
	require.Len(t, snap, 1)
	assert.Equal(t, inc.Key, snap[0].Key)
	assert.Equal(t, inc.Reason, snap[0].Reason)
	assert.Equal(t, inc.State, snap[0].State)

	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	restored := make(map[model.IncidentKey]*model.Incident, len(snap))
	for i := range snap {
		restored[snap[i].Key] = snap[i].ToIncident()
	}
	e2.RestoreIncidents(restored)
	assert.Equal(t, 1, e2.ActiveCount())
}

func TestMassFailurePersistenceRoundTrip(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	mfKey := MassFailureKey("configmap/ns/app-cfg")
	created := e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:       mfKey,
			Reason:    "CrashLoopBackOff",
			Namespace: "ns",
			Resource:  "pod",
			Name:      "configmap/ns/app-cfg",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	},
	)
	assert.True(t, created)
	assert.True(t, e.HasMassFailure(mfKey))
	// Duplicate add is a no-op.
	assert.False(t, e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key: mfKey,
		},
	}))

	snap := e.SnapshotPersisted()
	require.NotNil(t, snap)
	keys := make([]model.IncidentKey, 0, len(snap))
	for i := range snap {
		keys = append(keys, snap[i].Key)
	}
	assert.Contains(t, keys, mfKey)

	// Restore into a fresh engine WITHOUT any baseline: mass failures bypass
	// the baseline gate, so they survive restarts.
	e2 := NewEngine(Config{Window: 10 * time.Minute})
	restored := make(map[model.IncidentKey]*model.Incident, len(snap))
	for i := range snap {
		restored[snap[i].Key] = snap[i].ToIncident()
	}
	e2.RestoreIncidents(restored)
	assert.True(t, e2.HasMassFailure(mfKey))

	// Removing it clears the store.
	assert.True(t, e2.RemoveMassFailure(mfKey))
	assert.False(t, e2.HasMassFailure(mfKey))
	assert.False(t, e2.RemoveMassFailure(mfKey))
}

func TestMassFailureKeyHelpers(t *testing.T) {
	assert.True(t, IsMassFailureKey(MassFailureKey("node//n1")))
	assert.False(t, IsMassFailureKey("ns:dep:CrashLoopBackOff:"))

	parts := ParseKey(MassFailureKey("configmap/ns/app-cfg"))
	assert.True(t, parts.IsMassFailure)
	assert.Equal(t, "configmap/ns/app-cfg", parts.MassDependencyKey)

	// Non mass-failure keys parse as before.
	normal := ParseKey("ns:dep:CrashLoopBackOff:")
	assert.False(t, normal.IsMassFailure)
}
