package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

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

func TestSignalEventWithMessageHintFallback(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)
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
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		e,
		testAlertMgr,
	)

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
