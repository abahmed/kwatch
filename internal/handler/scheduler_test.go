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
)

func TestProcessPodUnschedulableDelay(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
		ScheduleMonitor:   config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: event lister error ---

func TestProcessPodUnschedulableWithResources(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec: corev1.PodSpec{
			NodeName: "node1",
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
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- OOMKilled with memory limit set (buildContainerHint line 195) ---

func TestProcessPodUnschedulableCreationDelay(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- emitHighRestartAlert early return when owner resolves to empty (RS owner, no rsLister) ---

func TestProcessPodUnschedulableDelayFallback(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h
	hh.now = func() time.Time { return time.Now().Add(-1 * time.Minute) }

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
	assert.NoError(t, hh.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- DetectControlPlanePodIssue: CrashLoopBackOff with LastTerminationState ---
