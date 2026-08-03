package handler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
)

func TestProcessContainerIgnoreLogPatternSuppresses(t *testing.T) {
	client := fake.NewSimpleClientset()

	// Control: without a log pattern the broken container alerts.
	e1 := testCorrelator()
	cfg1 := &config.Config{MaxRecentLogLines: 10}
	h1 := NewHandler(client, cfg1, e1, testAlertMgr)

	// Treatment: a pattern matching the logs must suppress the alert.
	e2 := testCorrelator()
	cfg2 := &config.Config{MaxRecentLogLines: 10}
	cfg2.Suppression = config.SuppressionIndex{
		LogPatterns: []*regexp.Regexp{regexp.MustCompile("fake logs")},
	}
	h2 := NewHandler(client, cfg2, e2, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "log-pod", Namespace: "default"},
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
				},
			},
		},
	}

	assert.NoError(t, h1.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e1.ActiveCount(), "control case should produce an incident")

	assert.NoError(t, h2.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e2.ActiveCount(), "log pattern suppression must skip the container")
}

func TestHighRestartCountIncident(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})

	cfg := &config.Config{
		ContainerRestartThreshold: 3,
	}
	h := NewHandler(fake.NewSimpleClientset(), cfg, e, testAlertMgr)

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
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Now(),
						},
					},
				},
			},
		},
	}

	ctx := &filter.Context{
		Ctx:    context.Background(),
		Client: fake.NewSimpleClientset(),
		Config: cfg,
		Pod:    pod,
	}

	h.executeContainersFilters(ctx)

	snap := e.Snapshot()
	var foundHighRestart bool
	for _, v := range snap {
		if v.Reason == "HighRestartCount" {
			foundHighRestart = true
			assert.Equal(t, model.StateActive, v.State)
			assert.Equal(t, "test-pod", v.Name)
		}
	}
	assert.True(t, foundHighRestart, "HighRestartCount incident should be created")
}
