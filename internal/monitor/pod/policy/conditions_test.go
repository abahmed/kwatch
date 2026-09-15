package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestContainerNameRuleIgnored(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}
	ctx.Runtime = config.RuntimeConfigFor(&config.Config{
		IgnoreContainerNames: []string{"test-container"},
	})

	rule := ContainerNameRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestContainerNameRuleNoMatch(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}
	ctx.Runtime = config.RuntimeConfigFor(&config.Config{
		IgnoreContainerNames: []string{"skip-container"},
	})

	rule := ContainerNameRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}

func TestContainerNameRuleEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{}),
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "test-container",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := ContainerNameRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}

func TestNoiseRuleEmptyReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{},
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	f := NoiseRule{}
	result := suppresses(f, ctx)
	assert.False(result)
}

func TestNoiseRuleNoiseReason(t *testing.T) {
	for _, reason := range NoiseReasons {
		t.Run(reason, func(t *testing.T) {
			assert := assert.New(t)

			ctx := &Context{
				Container: &ContainerContext{
					Container: &corev1.ContainerStatus{},
					Reason:    reason,
				},

				Pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
					},
				},
			}

			f := NoiseRule{}
			result := suppresses(f, ctx)
			assert.True(result)
		})
	}
}

func TestNoiseRuleNonNoiseReason(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Container: &ContainerContext{
			Container: &corev1.ContainerStatus{},
			Reason:    "CrashLoopBackOff",
		},

		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	f := NoiseRule{}
	result := suppresses(f, ctx)
	assert.False(result)
}
