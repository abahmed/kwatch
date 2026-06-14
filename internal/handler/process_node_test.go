package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProcessNodeReadyAndMemoryPressure(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	err := h.ProcessNodeObject(node, false)
	assert.NoError(t, err)

	snap := e.Snapshot()
	var foundMemoryPressure bool
	for _, v := range snap {
		if v.Reason == "MemoryPressure" {
			foundMemoryPressure = true
		}
	}
	assert.True(t, foundMemoryPressure, "MemoryPressure incident should exist")
}

func TestProcessNodeMemoryPressureResolve(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))

	node.Status.Conditions[0].Status = corev1.ConditionFalse
	assert.NoError(t, h.ProcessNodeObject(node, false))

	mu.Lock()
	res := resolves
	mu.Unlock()
	assert.Equal(t, 1, res, "MarkResolved should be called once")

	snap := e.Snapshot()
	for _, v := range snap {
		if v.Reason == "MemoryPressure" {
			assert.Equal(t, model.StateResolved, v.State, "MemoryPressure should be resolved")
		}
	}
}

func TestProcessNodeNotReadyUnknown(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))

	snap := e.Snapshot()
	var foundNodeNotReady bool
	for _, v := range snap {
		if v.Reason == "NodeNotReady" {
			foundNodeNotReady = true
			assert.Equal(t, model.StateActive, v.State)
		}
	}
	assert.True(t, foundNodeNotReady, "NodeNotReady incident should exist")
}

func TestProcessNodeMemoryPressureResolveIdempotent(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	// True → create incident
	assert.NoError(t, h.ProcessNodeObject(node, false))

	// False → resolve once
	node.Status.Conditions[0].Status = corev1.ConditionFalse
	assert.NoError(t, h.ProcessNodeObject(node, false))

	// False again → MUST NOT resolve again (idempotency)
	assert.NoError(t, h.ProcessNodeObject(node, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 1, r, "resolve must fire exactly once, not on every reconcile")
}

func TestProcessNodeHealthyNoResolve(t *testing.T) {
	var mu sync.Mutex
	var resolves int

	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
		LifecycleHook: func(inc *model.Incident, action model.IncidentAction) {
			mu.Lock()
			defer mu.Unlock()
			if action == model.ActionResolved {
				resolves++
			}
		},
	})

	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
			},
		},
	}

	// Reconciled 3× with no pressure — never fires resolve (no incident existed)
	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.NoError(t, h.ProcessNodeObject(node, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(t, 0, r, "no resolve should fire for a condition that was never True")
}
