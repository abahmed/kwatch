package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

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
