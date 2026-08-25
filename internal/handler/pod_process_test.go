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

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

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
