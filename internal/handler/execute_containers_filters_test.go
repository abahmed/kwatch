package handler

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

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
		Client: fake.NewSimpleClientset(),
		Config: cfg,
		Pod:    pod,
	}

	h.(*handler).executeContainersFilters(ctx)

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
