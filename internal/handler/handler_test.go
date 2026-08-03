package handler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
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

func TestNewHandlerWithMonitors(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 60},
		OomMonitor:        config.OomMonitor{Enabled: true, Threshold: 3, WindowMinutes: 10},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}

func TestNewHandlerPendingPodMonitorZeroThreshold(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 0},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NotNil(t, h)
}

func TestSignalEventWithRestartCountOnly(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.signalEvent(&event.Signal{
		Resource:     "pod",
		Reason:       "CrashLoopBackOff",
		PodName:      "p1",
		Namespace:    "ns1",
		RestartCount: 5,
	})
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodOOMKilledWithMemoryLimit(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "oom-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount(), "OOMKilled pod should create incident")
}

func TestProcessPodInitContainerError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "init-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "init-setup",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
}

func TestEmitHighRestartAlertWithOwner(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		ContainerRestartThreshold: 3,
	}, e, testAlertMgr)
	hh := h

	// Create a pod owned by a ReplicaSet to ensure owner resolution succeeds
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default"},
	}
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	f.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(rs)
	hh.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", APIVersion: "apps/v1"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap)
}

func TestProcessPodUnschedulableWithDelayAndResources(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}, e, testAlertMgr)
	hh := h
	hh.now = func() time.Time { return time.Now().Add(2 * time.Minute) }

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unscheduled-resources",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap, "Unschedulable pod with delay and resources should create incident")
}

func TestProcessPodWithEventLister(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Set event lister
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodImagePullBackOffNeedsRegistryAuth(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pull-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "gcr.io/myproject/myimage:v1",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image",
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodImagePullBackOffWithSecrets(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pull-pod-secrets", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "my-secret"},
			},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:latest",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image",
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodWithOomRepeating(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
		OomMonitor: config.OomMonitor{
			Enabled:       true,
			Threshold:     2,
			WindowMinutes: 10,
		},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "oom-repeat-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}

	// First call: records OOM
	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount(), "first OOM should create incident")

	// Second call: should detect OOM repeating
	err = h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount(), "second OOM should update existing incident")
}

func TestProcessPodWithCrashLoopBackOffLivenessProbe(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "liveness-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name: "app",
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/healthz",
								Port: intstr.FromInt(8080),
							},
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodUnschedulableWithEventLister(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h
	hh.now = func() time.Time { return time.Now().Add(2 * time.Minute) }

	// Set event lister to exercise that branch in executePodFilters
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unscheduled",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap)
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

func TestProcessDeploymentObjectUnavailable(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 0}, // fire immediately
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Seed deployment into informer
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       2,
			UnavailableReplicas: 1,
			UpdatedReplicas:     2,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "ReplicaSetUpdated",
				},
			},
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	// Process via string key (like the controller does)
	err := h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)

	// DeploymentUnavailable should fire as a signal event
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessDeploymentObjectUnavailableSustained(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 10}, // requires 10 min sustained
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       2,
			UnavailableReplicas: 1,
			UpdatedReplicas:     2,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	// First call: should NOT fire (not yet sustained)
	err := h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "should not fire before sustained window")

	// Second call with same deployment: still not sustained without time travel
	err = h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "still not sustained")
}

func TestProcessDeploymentObjectHealthy(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 2},
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Healthy deployment: all replicas ready/available
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "healthy-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       3,
			AvailableReplicas:   3,
			UnavailableReplicas: 0,
			UpdatedReplicas:     3,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/healthy-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "healthy deployment should not create incidents")
}

func TestProcessDeploymentObjectProgressDeadlineExceeded(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stuck-deploy",
			Namespace: "default",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "ProgressDeadlineExceeded",
				},
			},
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/stuck-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount(), "ProgressDeadlineExceeded should create an incident")
}

func TestProcessDeploymentObjectDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Process a deletion
	err := h.ProcessDeployment("default/missing-deploy", true)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessDeploymentObjectDirectDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessDeploymentObject(deploy, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessDeploymentObjectNotYetObserved(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 0},
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Generation mismatch: ObservedGeneration < Generation
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "new-deploy",
			Namespace:  "default",
			Generation: 3,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			UnavailableReplicas: 3,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/new-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "not yet observed deployment should not fire")
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

func TestBuildProbeHintLivenessHTTP(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(8080),
				},
			},
		},
	}
	hint := buildProbeHint("LivenessProbeFailed", spec)
	assert.Contains(t, hint, "HTTP GET")
	assert.Contains(t, hint, "liveness")
}

func TestBuildProbeHintReadinessTCP(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(3306),
				},
			},
		},
	}
	hint := buildProbeHint("ReadinessProbeFailed", spec)
	assert.Contains(t, hint, "TCP check")
	assert.Contains(t, hint, "readiness")
}

func TestBuildProbeHintStartupExec(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/check", "--ready"},
				},
			},
		},
	}
	hint := buildProbeHint("StartupProbeFailed", spec)
	assert.Contains(t, hint, "exec")
	assert.Contains(t, hint, "startup")
}

func TestBuildProbeHintNilProbe(t *testing.T) {
	spec := &corev1.Container{Name: "app"}
	hint := buildProbeHint("LivenessProbeFailed", spec)
	assert.NotContains(t, hint, "(HTTP")
}

func TestProbeType(t *testing.T) {
	assert.Equal(t, "liveness", probeType("LivenessProbeFailed"))
	assert.Equal(t, "readiness", probeType("ReadinessProbeFailed"))
	assert.Equal(t, "startup", probeType("StartupProbeFailed"))
	assert.Equal(t, "probe", probeType("Unknown"))
	assert.Equal(t, "probe", probeType(""))
}

func TestImagePullMsgHintRateLimit(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("toomanyrequests: pull limit", false), "rate limit")
	assert.Contains(t, imagePullMsgHint("rate limit exceeded", false), "rate limit")
}

func TestImagePullMsgHintPullQPS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("pull qps exceeded", false), "QPS")
}

func TestImagePullMsgHintAuth(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("authentication required", false), "authentication")
	assert.Contains(t, imagePullMsgHint("unauthorized: access denied", false), "authentication")
	assert.Contains(t, imagePullMsgHint("denied: access forbidden", false), "authentication")
	assert.Contains(t, imagePullMsgHint("no pull access", false), "authentication")
}

func TestImagePullMsgHintNotFound(t *testing.T) {
	withSecrets := imagePullMsgHint("not found: nginx:latest", true)
	assert.Contains(t, withSecrets, "not found")
	assert.Contains(t, withSecrets, "registry")
	withoutSecrets := imagePullMsgHint("manifest unknown", false)
	assert.Contains(t, withoutSecrets, "not found")
	assert.NotContains(t, withoutSecrets, "registry")
}

func TestImagePullMsgHintTimeout(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("context deadline exceeded", false), "timed out")
	assert.Contains(t, imagePullMsgHint("i/o timeout", false), "timed out")
}

func TestImagePullMsgHintConnRefused(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("connection refused", false), "refused")
	assert.Contains(t, imagePullMsgHint("connection reset", false), "refused")
}

func TestImagePullMsgHintNoRoute(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("no route to host", false), "route")
	assert.Contains(t, imagePullMsgHint("network is unreachable", false), "route")
}

func TestImagePullMsgHintDNS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("no such host", false), "DNS")
	assert.Contains(t, imagePullMsgHint("dial tcp: lookup registry.example.com", false), "DNS")
}

func TestImagePullMsgHintTLS(t *testing.T) {
	assert.Contains(t, imagePullMsgHint("tls handshake error", false), "TLS")
	assert.Contains(t, imagePullMsgHint("certificate expired", false), "TLS")
}

func TestImagePullMsgHintNoMatch(t *testing.T) {
	assert.Equal(t, "", imagePullMsgHint("some random error", false))
}

func TestNeedsRegistryAuth(t *testing.T) {
	assert.False(t, needsRegistryAuth("nginx"))
	assert.False(t, needsRegistryAuth("nginx:latest"))
	assert.False(t, needsRegistryAuth("library/nginx"))
	assert.False(t, needsRegistryAuth("myuser/myimage"))
	assert.True(t, needsRegistryAuth("gcr.io/myproject/myimage"))
	assert.True(t, needsRegistryAuth("myregistry.io:5000/myimage"))
	assert.True(t, needsRegistryAuth("docker.io/user/repo"))
	assert.False(t, needsRegistryAuth(""))
}

func TestIsPodTerminatingOrDisrupted(t *testing.T) {
	assert.False(t, isPodTerminatingOrDisrupted(nil))
	assert.False(t, isPodTerminatingOrDisrupted(&corev1.Pod{}))

	now := metav1.Now()
	assert.True(t, isPodTerminatingOrDisrupted(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
	}))

	assert.True(t, isPodTerminatingOrDisrupted(&corev1.Pod{
		Status: corev1.PodStatus{
			Phase:  corev1.PodFailed,
			Reason: "Evicted",
		},
	}))

	assert.True(t, isPodTerminatingOrDisrupted(&corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: "DisruptionTarget"},
			},
		},
	}))
}

func TestRoundDuration(t *testing.T) {
	assert.Equal(t, "5s", roundDuration(5*time.Second))
	assert.Equal(t, "0s", roundDuration(0))
	assert.Equal(t, "1m0s", roundDuration(time.Minute))
	assert.Equal(t, "2m30s", roundDuration(150*time.Second))
	assert.Equal(t, "1h0m", roundDuration(time.Hour))
	assert.Equal(t, "2h15m", roundDuration(2*time.Hour+15*time.Minute))
}

func TestLastTermInfo(t *testing.T) {
	reason, code := lastTermInfo(&corev1.ContainerStatus{})
	assert.Equal(t, "", reason)
	assert.Equal(t, int32(0), code)

	reason, code = lastTermInfo(&corev1.ContainerStatus{
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason:   "OOMKilled",
				ExitCode: 137,
			},
		},
	})
	assert.Equal(t, "OOMKilled", reason)
	assert.Equal(t, int32(137), code)
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

func TestProcessDeploymentInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessDeployment("a/b/c", false))
}

func TestProcessDeploymentNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	assert.NoError(t, h.ProcessDeployment("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestSetReplicaLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())
	assert.NotNil(t, h.rsLister)
}

func TestSetSecretLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetSecretLister(f.Core().V1().Secrets().Lister())
	assert.NotNil(t, h.secretLister)
}

func TestSetInsightEngine(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetInsightEngine(nil)
	assert.Nil(t, h.insightEngine)
}

func TestProcessNodeResourceOvercommit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.ProcessNodeResourceOvercommit("NodeMemoryPressure", "node1", "high memory usage", "critical")
	assert.Equal(t, 1, e.ActiveCount())
}

func TestSetSeen(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetSeen(map[string]map[string]int64{"default/test-pod": {"CrashLoopBackOff": 100}})
}

func TestSetActiveNodeIncidents(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.SetActiveNodeIncidents([]string{"node1", "node2"})
}

func TestReportStartupSummaryNoSuppressed(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	h.ReportStartupSummary(nil)
}

func TestReportStartupSummaryWithData(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		ReportStartupBaseline: true,
	}, testCorrelator(), testAlertMgr)
	h.ReportStartupSummary(map[string]int{"CrashLoopBackOff": 3, "ImagePullBackOff": 2})
}

func TestEmitHighRestartAlertNoOwner(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		},
	}
	h.emitHighRestartAlert(ctx, &corev1.ContainerStatus{
		Name:         "c1",
		RestartCount: 5,
	})
}

func TestFindContainerSpec(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
			InitContainers: []corev1.Container{
				{Name: "init-setup"},
			},
		},
	}
	assert.NotNil(t, findContainerSpec(pod, "app"))
	assert.NotNil(t, findContainerSpec(pod, "sidecar"))
	assert.NotNil(t, findContainerSpec(pod, "init-setup"))
	assert.Nil(t, findContainerSpec(pod, "nonexistent"))
}

func TestProcessPodUnschedulableWithScheduleMonitor(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)
	h.now = func() time.Time { return time.Now().Add(time.Minute) }

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unscheduled",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.PodSpec{NodeName: ""},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionFalse,
					Reason: "Unschedulable",
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap, "Unschedulable pod should create incident")
}

func TestProcessPodUnschedulableWithResourceRequests(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "req-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionFalse,
					Reason: "Unschedulable",
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap, "Unschedulable pod should create incident")
}

func TestSignalEventWithInsightEngine(t *testing.T) {
	e := testCorrelator()
	ie := insight.Engine{}
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.SetInsightEngine(&ie)
	h.signalEvent(&event.Signal{
		Resource:  "pod",
		Reason:    "TestReason",
		PodName:   "p1",
		Namespace: "ns1",
	})
	assert.Equal(t, 1, e.ActiveCount())
}

func TestSignalEventWithMessageHintFallback(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	h.signalEvent(&event.Signal{
		Resource:  "pod",
		Reason:    "TestReason",
		PodName:   "p1",
		Namespace: "ns1",
		Message:   "fallback message",
	})
	assert.Equal(t, 1, e.ActiveCount())
}

func TestSignalEventWithContainerState(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	h.signalEvent(&event.Signal{
		Resource:     "pod",
		Reason:       "TestReason",
		PodName:      "p1",
		Namespace:    "ns1",
		RestartCount: 3,
		ContainerState: &model.ContainerState{
			RestartCount: 3,
			Reason:       "OOMKilled",
			ExitCode:     137,
		},
	})
	assert.Equal(t, 1, e.ActiveCount())
}
