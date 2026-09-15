package policy

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodNameRule(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^test-.*"),
		},
	}

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(cfg),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := PodNameRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestPodNameRuleNoMatch(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{
			regexp.MustCompile("^skip-.*"),
		},
	}

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(cfg),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := PodNameRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}

func TestPodNameRuleEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{}

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(cfg),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	rule := PodNameRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}
