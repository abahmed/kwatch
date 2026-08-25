package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestProcessNodeNilObject(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessNodeObject(nil, false))
}

func TestProcessNodeDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, true))
}

func TestProcessNodeNotReadyNoAlert(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		NodeReasons:  []string{"KubeletNotReady"},
		NodeMessages: []string{"specific message"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
}

func TestProcessNodeReadyRecovery(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
					Reason: "KubeletReady",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
}

func TestProcessNodeNotReadyAlert(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
}

func TestProcessNodeRecoveryClearsStaleInhibition(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	e := correlation.NewEngine(correlation.Config{
		Window:                    10 * time.Minute,
		InhibitNodeSuppressesPods: true,
	})
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Node was NotReady before startup — baselined, flag pre-seeded, no incident.
	e.SetActiveNodeIncidents([]string{"test-node"})

	// Node recovers — the handler must clear the stale inhibition flag even
	// though there is no incident in state to resolve.
	healthy := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	assert.NoError(t, h.ProcessNodeObject(healthy, false))

	// A subsequent pod incident on that node must alert normally.
	inc, action := e.Process(event.Event{PodName: "p", Namespace: "ns", NodeName: "test-node", Reason: "CrashLoopBackOff"}, "dep", nil)
	assert.Equal(t, model.ActionCreate, action)
	assert.NotNil(t, inc)
}

func TestProcessNodeNotReadyWithIgnoredMessage(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		NodeMessages: []string{"draining"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "NodeNotReady",
					Message: "node is draining for maintenance",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
}

func TestProcessNodeAlreadyKnownNotReady(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessNodeObject(node, false))
}

func TestProcessNodeResourceOvercommit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.ProcessNodeResourceOvercommit("NodeMemoryPressure", "node1", "high memory usage", "critical")
	assert.Equal(t, 1, e.ActiveCount())
}
