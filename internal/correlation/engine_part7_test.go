package correlation

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestMassFailureSetClone(t *testing.T) {
	e := NewEngine(Config{Window: 10 * time.Minute})
	e.AddMassFailure(&model.Incident{
		Subject: model.Subject{
			Key:      MassFailureKey("node//n1"),
			Reason:   "NotReady",
			Resource: "node",
		},
		Status: model.Status{
			State: model.StateActive,
		},
	})

	snap := e.MassFailureSet()
	require.Len(t, snap, 1)
	// Mutating the snapshot must not corrupt the store.
	for _, inc := range snap {
		inc.State = model.StateResolved
	}
	assert.True(t, e.HasMassFailure(MassFailureKey("node//n1")))
}

func TestSnapshotAllEmpty(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	snap := e.SnapshotAll()
	// No incidents processed so dirty=false and SnapshotAll returns nil
	assert.Nil(t, snap)
}

func TestActiveIncidentsDoesNotClearDirty(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{
		PodName:   "pod-1",
		Namespace: "ns",
		Reason:    "CrashLoopBackOff",
	}
	_, _ = e.Process(ev, "dep", &model.ContainerState{RestartCount: 1})

	// Multiple consumers within a tick: ActiveIncidents must behave the same
	// for every call AND leave SnapshotAll able to report the incident.
	first := e.ActiveIncidents()
	require.Len(t, first, 1)
	second := e.ActiveIncidents()
	require.Len(t, second, 1)
	assert.Equal(t, first, second)

	snap := e.SnapshotAll()
	require.Len(t, snap, 1)
}

func TestRestoreIncidentsBumpsLastSeen(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	ev := event.Event{PodName: "pod-1", Namespace: "ns", Reason: "OOMKilled"}
	inc, _ := e.Process(ev, "dep", &model.ContainerState{RestartCount: 1})
	require.NotNil(t, inc)
	originalLastSeen := inc.LastSeen

	time.Sleep(time.Millisecond)

	snap := e.SnapshotAll()
	e2 := NewEngine(Config{
		Window: 10 * time.Minute,
		Baseline: map[string]map[string]int64{
			string(inc.Key): {"pod-1": time.Now().Unix()},
		},
	})
	e2.RestoreIncidents(snap)

	e2.mu.Lock()
	restored, exists := e2.state[inc.Key]
	e2.mu.Unlock()
	assert.True(t, exists, "restored incident must exist in state")
	assert.True(
		t,
		restored.LastSeen.After(originalLastSeen),
		"expected restored LastSeen (%v) to be after original (%v)",
		restored.LastSeen,
		originalLastSeen,
	)
}

func TestIsOwnerHealthyDeploymentHealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}

	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  2,
					Replicas:            3,
					ReadyReplicas:       2,
					UnavailableReplicas: 1,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentNotObserved(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  1,
					Replicas:            3,
					ReadyReplicas:       3,
					UnavailableReplicas: 0,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDeploymentNotFound(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDeployLister(&mockDeployLister{
		getFn: func(ns, name string) (*appsv1.Deployment, error) {
			return nil, fmt.Errorf("not found")
		},
	})

	// With resources → unhealthy (keep incident open)
	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
		Status: model.Status{
			Resources: map[string]bool{"p": true},
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))

	// Without resources → healthy (safe to resolve)
	inc2 := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc2))
}

func TestIsOwnerHealthyNonPodResource(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-node",
			OwnerKind: "",
			Resource:  "node",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyNilListers(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-deploy",
			OwnerKind: "Deployment",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyStatefulSet(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetStatefulSetLister(&mockSSLister{
		getFn: func(ns, name string) (*appsv1.StatefulSet, error) {
			return &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 2,
					Replicas:           3,
					ReadyReplicas:      3,
					CurrentRevision:    "rev-2",
					UpdateRevision:     "rev-2",
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ss",
			OwnerKind: "StatefulSet",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDaemonSet(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDaemonSetLister(&mockDSLister{
		getFn: func(ns, name string) (*appsv1.DaemonSet, error) {
			return &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberUnavailable:      0,
					UpdatedNumberScheduled: 3,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ds",
			OwnerKind: "DaemonSet",
			Resource:  "pod",
		},
	}
	assert.True(t, e.isOwnerHealthy(inc))
}

func TestIsOwnerHealthyDaemonSetUnhealthy(t *testing.T) {
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.SetDaemonSetLister(&mockDSLister{
		getFn: func(ns, name string) (*appsv1.DaemonSet, error) {
			return &appsv1.DaemonSet{
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: 3,
					NumberUnavailable:      1,
					UpdatedNumberScheduled: 2,
				},
			}, nil
		},
	})

	inc := &model.Incident{
		Subject: model.Subject{
			Namespace: "ns",
			Name:      "my-ds",
			OwnerKind: "DaemonSet",
			Resource:  "pod",
		},
	}
	assert.False(t, e.isOwnerHealthy(inc))
}

func TestClearBaselineForPodClearsCooldown(t *testing.T) {
	fakeNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEngine(Config{
		Window: 10 * time.Minute,
	})
	e.now = mockClock(fakeNow)

	key := BuildKey("ns", "dep", "CrashLoopBackOff", "")

	// Manually add cooldown entry
	e.mu.Lock()
	e.cleanupCooldown[key] = fakeNow.Add(10 * time.Minute)
	e.mu.Unlock()

	// ClearBaselineForPod for the pod's namespace
	e.ClearBaselineForPod("ns", "pod-1")

	// Cooldown should be cleared
	e.mu.Lock()
	_, exists := e.cleanupCooldown[key]
	e.mu.Unlock()
	assert.False(t, exists, "cooldown should be cleared by ClearBaselineForPod")
}

// Smart grouping (reason-adaptive) tests.
