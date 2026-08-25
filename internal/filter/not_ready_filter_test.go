package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func notReadyPod() *corev1.Pod {
	transition := metav1.Now().Add(-2 * time.Minute)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.Time{Time: transition},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: false,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}
}

func TestNotReadyFilterAlertsAfterThreshold(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{},
		Pod:    notReadyPod(),
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusAlert, f.Detect(ctx))
	assert.True(t, ctx.PodHasIssues)
	assert.False(t, ctx.ContainersHasIssues)
	assert.Equal(t, constant.ReasonContainersNotReady, ctx.PodReason)
}

func TestNotReadyFilterSilentBeforeThreshold(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{},
		Pod:    notReadyPod(),
	}
	// Threshold greater than the time since last transition → not yet.
	f := NotReadyFilter{Threshold: 5 * time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
	assert.False(t, ctx.PodHasIssues)
}

func TestNotReadyFilterSilentWhenReady(t *testing.T) {
	pod := notReadyPod()
	pod.Status.Conditions[0].Status = corev1.ConditionTrue
	ctx := &Context{
		Config: &config.Config{},
		Pod:    pod,
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
}

func TestNotReadyFilterSkipsCrashingContainer(t *testing.T) {
	pod := notReadyPod()
	pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
	}
	ctx := &Context{
		Config: &config.Config{},
		Pod:    pod,
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
}

func TestNotReadyFilterSkipsNonRunningPhase(t *testing.T) {
	pod := notReadyPod()
	pod.Status.Phase = corev1.PodPending
	ctx := &Context{
		Config: &config.Config{},
		Pod:    pod,
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
}

func TestNotReadyFilterSkipsWhenAlreadyAlerted(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{},
		Pod:    notReadyPod(),
		PodLastState: &model.ContainerState{
			Reason: constant.ReasonContainersNotReady,
		},
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
}

func TestNotReadyFilterRespectsWatchStartTime(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{
			WatchStartTime: time.Now(),
		},
		Pod: notReadyPod(),
	}
	f := NotReadyFilter{Threshold: time.Minute}
	assert.Equal(t, StatusContinue, f.Detect(ctx))
}
