package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestIsPodHealthyRunningNoIssues(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	assert.True(t, isPodHealthy(pod))
}

func TestIsPodHealthyWaitingIssue(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}
	assert.False(t, isPodHealthy(pod))
}

func TestIsPodHealthyContainerCreating(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ContainerCreating",
						},
					},
				},
			},
		},
	}
	assert.True(t, isPodHealthy(pod))
}

func TestIsPodHealthyTerminatedNonZero(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "Error",
						},
					},
				},
			},
		},
	}
	assert.False(t, isPodHealthy(pod))
}

func TestIsPodHealthyTerminatedCompleted(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Reason:   "Completed",
						},
					},
				},
			},
		},
	}
	assert.True(t, isPodHealthy(pod))
}

func TestIsPodHealthyNotRunning(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	assert.False(t, isPodHealthy(pod))
}

func TestProcessPodObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessPodObject(context.Background(), nil, false))
}

func TestProcessPodObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, true))
}

func TestProcessPodObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodInvalidKey(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.Error(t, h.ProcessPod(context.Background(), "a/b/c", false))
}

func TestProcessPodTopLevelDeleted(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)
	assert.NoError(t, h.ProcessPod(context.Background(), "ns1/p1", true))
}

func TestProcessPodNotFound(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{})
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	h.listers.Pod = f.Core().V1().Pods().Lister()
	assert.NoError(t, h.ProcessPod(context.Background(), "ns1/missing", false))
}
