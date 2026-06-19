package handler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var testAlertMgr = &alert.AlertManager{}

func testCorrelator() *correlation.Engine {
	return correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})
}

func TestNewHandler(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}

func TestProcessPodNilObject(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessPodObject(context.Background(), nil, false))
}

func TestProcessPodDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, true))
}

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

func TestProcessPodWithPodIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodWithContainersIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "test-container",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "container is crashing",
						},
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ForbiddenNamespaces: []string{"kube-system"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredPodName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{regexp.MustCompile("^test-.*")},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredContainerName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	cfg.Suppression = config.SuppressionIndex{
		ContainerNames: []string{"test-container"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "test-container",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "container is crashing",
						},
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestHealthyPodZeroAPICalls(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
					},
				},
			},
		},
	}

	startCount := len(client.Fake.Actions())
	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	endCount := len(client.Fake.Actions())

	assert.Equal(t, startCount, endCount, "healthy pod should not trigger any API calls")
}

func TestBrokenPodMakesAPICalls(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
							Message:  "memory limit exceeded",
						},
					},
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}

	startCount := len(client.Fake.Actions())
	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	endCount := len(client.Fake.Actions())

	// Without event lister: 1 event LIST + 1 log GET = 2 API calls
	assert.Equal(t, 2, endCount-startCount, "broken pod should trigger exactly 2 API calls (1 event LIST + 1 log GET)")
}

func TestBrokenPodEventsFromCache(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
	}
	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Seed event lister with an event for this pod
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
							Message:  "memory limit exceeded",
						},
					},
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}

	startCount := len(client.Fake.Actions())
	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	endCount := len(client.Fake.Actions())

	// With event lister: 0 event LISTs + 1 log GET = 1 API call
	assert.Equal(t, 1, endCount-startCount, "broken pod with event lister should trigger exactly 1 API call (log GET only)")
}

func TestProcessPodSucceededPhase(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodCompletedStatus(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
					Reason: "PodCompleted",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}
