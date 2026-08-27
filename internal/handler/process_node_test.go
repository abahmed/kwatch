package handler

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/model"
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

	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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

	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
			assert.Equal(
				t,
				model.StateResolved,
				v.State,
				"MemoryPressure should be resolved",
			)
		}
	}
}

func TestProcessNodeNotReadyUnknown(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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

	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
	assert.Equal(
		t,
		1,
		r,
		"resolve must fire exactly once, not on every reconcile",
	)
}

func TestNodeConditionReason(t *testing.T) {
	assert.Equal(
		t,
		"",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		),
	)
	assert.Equal(
		t,
		"NodeNotReady",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		),
	)
	assert.Equal(
		t,
		"NodeNotReady",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionUnknown,
			},
		),
	)
	assert.Equal(
		t,
		"MemoryPressure",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeMemoryPressure,
				Status: corev1.ConditionTrue,
			},
		),
	)
	assert.Equal(
		t,
		"DiskPressure",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeDiskPressure,
				Status: corev1.ConditionTrue,
			},
		),
	)
	assert.Equal(
		t,
		"PIDPressure",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodePIDPressure,
				Status: corev1.ConditionTrue,
			},
		),
	)
	assert.Equal(
		t,
		"NetworkUnavailable",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeNetworkUnavailable,
				Status: corev1.ConditionTrue,
			},
		),
	)
	assert.Equal(
		t,
		"",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   corev1.NodeMemoryPressure,
				Status: corev1.ConditionFalse,
			},
		),
	)
	assert.Equal(
		t,
		"",
		NodeConditionReason(
			corev1.NodeCondition{
				Type:   "FakeCondition",
				Status: corev1.ConditionTrue,
			},
		),
	)
}

func TestProcessNodeKeyDeleted(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessNode("test-node", true))
}

func TestProcessNodeNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Node = f.Core().V1().Nodes().Lister()

	assert.NoError(t, h.ProcessNode("missing-node", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessNodeKeyValid(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.listers.Node = f.Core().V1().Nodes().Lister()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
			},
		},
	}
	f.Core().V1().Nodes().Informer().GetIndexer().Add(node)

	assert.NoError(t, h.ProcessNode("test-node", false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "NodeNotReady" {
			found = true
		}
	}
	assert.True(t, found, "key-based ProcessNode should create incident")
}

func TestProcessNodeNewNodeSkipsAlert(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "new-node",
			CreationTimestamp: metav1.Now(),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"new node NotReady should not create incident",
	)
}

func TestProcessNodeDeletingSkipsAlert(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	now := metav1.Now()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "del-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			DeletionTimestamp: &now,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"deleting node NotReady should not create incident",
	)
}

func TestProcessNodeUnschedulableSkipsAlert(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unschedulable-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Spec: corev1.NodeSpec{Unschedulable: true},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"unschedulable node NotReady should not create incident",
	)
}

func TestProcessNodeDiskPressure(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "dp-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "DiskPressure" {
			found = true
		}
	}
	assert.True(t, found, "DiskPressure incident should exist")
}

func TestProcessNodePIDPressure(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pid-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "PIDPressure" {
			found = true
		}
	}
	assert.True(t, found, "PIDPressure incident should exist")
}

func TestProcessNodeNetworkUnavailable(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "net-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeNetworkUnavailable,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))

	snap := e.Snapshot()
	var found bool
	for _, v := range snap {
		if v.Reason == "NetworkUnavailable" {
			found = true
		}
	}
	assert.True(t, found, "NetworkUnavailable incident should exist")
}

func TestProcessNodePressureSustained(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		NodeMonitor: config.NodeMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pressure-node",
			CreationTimestamp: metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.Equal(
		t,
		0,
		e.ActiveCount(),
		"pressure should not fire within sustained window",
	)
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

	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeMemoryPressure,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	// Reconciled 3× with no pressure — never fires resolve (no incident
	// existed)
	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.NoError(t, h.ProcessNodeObject(node, false))
	assert.NoError(t, h.ProcessNodeObject(node, false))

	mu.Lock()
	r := resolves
	mu.Unlock()
	assert.Equal(
		t,
		0,
		r,
		"no resolve should fire for a condition that was never True",
	)
}

func TestMarkFirstNodePressureHit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	t1 := h.markFirstNodePressure("node1/DiskPressure")
	t2 := h.markFirstNodePressure("node1/DiskPressure")
	assert.Equal(t, t1, t2, "second call should return existing entry")
}

func TestClearAllNodePressureNoMatch(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	h.fs.nodePressure.seed("node1/DiskPressure", time.Now())
	h.fs.nodePressure.seed("node1/PIDPressure", time.Now())
	h.clearAllNodePressure("other-node")
	assert.Len(
		t,
		h.fs.nodePressure.dump(),
		2,
		"should not clear entries for other node",
	)
}

func TestClearAllNodePressureWithMatch(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	h.fs.nodePressure.seed("node1/DiskPressure", time.Now())
	h.fs.nodePressure.seed("node1/PIDPressure", time.Now())
	h.fs.nodePressure.seed("node2/MemoryPressure", time.Now())
	h.clearAllNodePressure("node1")
	assert.Len(
		t,
		h.fs.nodePressure.dump(),
		1,
		"should clear node1 entries only",
	)
	assert.Contains(t, h.fs.nodePressure.dump(), "node2/MemoryPressure")
}
