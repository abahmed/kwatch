package incident

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

func TestSmartGroupingImageAuth(t *testing.T) {
	e := newFanOutGroupingEngine()
	msg := "unauthorized: authentication required"
	ev := event.Event{
		PodName: "p1", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "nginx:latest", Message: msg,
	}
	e.processEvent(ev, "dep1", nil)
	ev2 := event.Event{
		PodName: "p2", Namespace: "ns", Reason: "ImagePullBackOff",
		Image: "alpine:latest", Message: msg,
	}
	e.processEvent(ev2, "dep2", nil)

	e.mu.Lock()
	gk := "ImagePullBackOff|ns|ns"
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "auth ns-scoped group must exist")
	assert.Equal(t, 2, len(pg.entries))
}

func TestSmartGroupingNamespaceScope(t *testing.T) {
	e := newFanOutGroupingEngine()
	e.processEvent(
		event.Event{
			PodName:   "p1",
			Namespace: "ns",
			Reason:    "CreateContainerConfigError",
		},
		"dep1",
		nil,
	)
	e.processEvent(
		event.Event{
			PodName:   "p2",
			Namespace: "ns2",
			Reason:    "CreateContainerConfigError",
		},
		"dep2",
		nil,
	)

	e.mu.Lock()
	_, has1 := e.groupBuffers["CreateContainerConfigError|ns|ns"]
	_, has2 := e.groupBuffers["CreateContainerConfigError|ns|ns2"]
	e.mu.Unlock()
	assert.True(t, has1, "ns group must exist")
	assert.True(t, has2, "ns2 group must exist")
}

func TestSmartGroupingCrossNamespace(t *testing.T) {
	e := newSmartGroupingEngine()
	// Each namespace has its own window, so each owner is the first in its
	// namespace and both alert immediately; nothing is buffered.
	_, a1 := e.processEvent(
		event.Event{PodName: "p1", Namespace: "ns1", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	_, a2 := e.processEvent(
		event.Event{PodName: "p2", Namespace: "ns2", Reason: "OOMKilled"},
		"dep1",
		nil,
	)
	assert.Equal(t, model.ActionCreate, a1)
	assert.Equal(t, model.ActionCreate, a2)
	e.mu.Lock()
	buffers := len(e.groupBuffers)
	e.mu.Unlock()
	assert.Equal(t, 0, buffers, "a lone owner per namespace is never buffered")
}

func TestSmartGroupingEntryLimit(t *testing.T) {
	e := newSmartGroupingEngine()
	gk := "ImagePullBackOff|ns|ns"

	for i := 0; i < 1002; i++ {
		ev := event.Event{
			PodName:   fmt.Sprintf("p%d", i),
			Namespace: "ns",
			Reason:    "ImagePullBackOff",
			Image:     fmt.Sprintf("image:%d", i),
			Message:   "unauthorized: authentication required",
		}
		e.processEvent(ev, fmt.Sprintf("dep%d", i), nil)
	}

	e.mu.Lock()
	pg, ok := e.groupBuffers[gk]
	e.mu.Unlock()
	require.True(t, ok, "pending group must exist")
	assert.Equal(t, maxGroupEntries, len(pg.entries), "entries must be capped")
	assert.Equal(
		t,
		2,
		pg.overflowCount,
		"1 entry from first overflow + 1 from second",
	)
}

func TestSmartGroupingSeverityInheritance(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := newFanOutGroupingEngine()
	e.now = mockClock(now)

	sigLog := "connection refused:5432"
	e.processEvent(event.Event{
		PodName: "p1", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: sigLog, Severity: "normal",
	}, "dep1", nil)
	e.processEvent(event.Event{
		PodName: "p2", Namespace: "ns", Reason: "CrashLoopBackOff",
		Logs: sigLog, Severity: "critical",
	}, "dep2", nil)

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
	assert.Equal(
		t,
		model.SeverityCritical,
		groupInc.Severity,
		"group must inherit highest severity",
	)
}

// --- cascading suppression ---

func TestCascadingSuppressionSuppressesPodWhenDeploymentUnavailable(
	t *testing.T,
) {
	e := newTestEngine(Config{
		Window: 10 * time.Minute,
	})
	setTestDeployLister(e, &mockDeployLister{
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

	// First, create a deployment-level incident
	depEv := event.Event{
		Resource:  "deployment",
		Namespace: "ns",
		PodName:   "myapp",
		Reason:    "DeploymentUnavailable",
	}
	depInc, depAction := e.processEvent(depEv, "myapp", nil)
	assert.Equal(t, model.ActionCreate, depAction)
	assert.NotNil(t, depInc)

	// Now process a pod incident for the same owner
	podEv := event.Event{
		Resource:      "pod",
		Namespace:     "ns",
		PodName:       "myapp-7d8f9-abc",
		ContainerName: "app",
		Reason:        "CrashLoopBackOff",
	}
	podInc, podAction := e.processEvent(
		podEv,
		"myapp",
		&model.ContainerState{RestartCount: 1},
	)
	assert.Equal(
		t,
		model.ActionSkip,
		podAction,
		"pod incident should be suppressed",
	)
	assert.Nil(t, podInc)

	// Verify the deployment incident was attributed
	assert.Equal(t, 2, e.state[depInc.Key].Count)
	assert.Equal(t, 1, e.state[depInc.Key].SuppressedPods)
	assert.False(t, e.state[depInc.Key].Resources["myapp-7d8f9-abc"])
}

func TestCascadingSuppressionNoSuppressionWhenDeploymentHealthy(t *testing.T) {
	e := newTestEngine(Config{
		Window: 10 * time.Minute,
	})
	setTestDeployLister(e, &mockDeployLister{
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

	// Create a pod incident (no parent incident exists)
	ev := event.Event{
		Resource:  "pod",
		Namespace: "ns",
		PodName:   "myapp-7d8f9-abc",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.processEvent(ev, "myapp", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
}
