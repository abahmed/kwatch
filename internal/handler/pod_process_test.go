package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
)

func TestSignalEventWithRestartCountOnly(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
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
	hh.listers.RS = f.Apps().V1().ReplicaSets().Lister()

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
	assert.NotEmpty(
		t,
		snap,
		"Unschedulable pod with delay and resources should create incident",
	)
}

func TestProcessPodWithEventLister(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Set event lister
	f := informers.NewSharedInformerFactory(client, 0)
	h.listers.Event = f.Core().V1().Events().Lister()

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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-pod-secrets",
			Namespace: "default",
		},
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
