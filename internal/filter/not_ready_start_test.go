package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

// PodReady=False is stamped at scheduling, but on a cold node the application
// container starts a minute later, after init containers and an image pull. A
// pod that has never been ready is measured from when its container actually
// started, or every such rollout alerts for a process that is seconds old.
func TestNotReadyFilterMeasuresFirstStartFromContainerStart(t *testing.T) {
	now := time.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: now.Add(-90 * time.Second)},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
				LastTransitionTime: metav1.Time{
					Time: now.Add(-90 * time.Second),
				},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.Time{Time: now.Add(-20 * time.Second)},
					},
				},
			}},
		},
	}
	ctx := &Context{Sources: Sources{Config: &config.Config{}}, Pod: pod}
	f := NotReadyFilter{Threshold: time.Minute}

	assert.Equal(t, StatusContinue, f.Detect(ctx),
		"twenty seconds after the container started is not a minute unready")
}

func TestNotReadyFilterStillAlertsWhenContainerStartedLongAgo(t *testing.T) {
	now := time.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: now.Add(-5 * time.Minute)},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
				LastTransitionTime: metav1.Time{
					Time: now.Add(-5 * time.Minute),
				},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.Time{Time: now.Add(-4 * time.Minute)},
					},
				},
			}},
		},
	}
	ctx := &Context{Sources: Sources{Config: &config.Config{}}, Pod: pod}
	f := NotReadyFilter{Threshold: time.Minute}

	assert.Equal(t, StatusAlert, f.Detect(ctx))
	assert.Contains(t, ctx.PodMsg, "never become ready")
}

func TestLogsUnavailableRecognisesKubeletSentinel(t *testing.T) {
	assert.True(t, logsUnavailable(
		"unable to retrieve container logs for containerd://cffc1f44",
	))
	assert.True(t, logsUnavailable(
		"\nunable to retrieve container logs for docker://abc\n",
	))
	assert.False(t, logsUnavailable("panic: runtime error"))
	assert.False(t, logsUnavailable(""))
}
