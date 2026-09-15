package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestNamespaceRuleAllowed(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{
		AllowedNamespaces: []string{"default", "kube-system"},
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

	rule := NamespaceRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}

func TestNamespaceRuleForbidden(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{
		ForbiddenNamespaces: []string{"kube-system"},
	}

	ctx := &Context{
		Runtime: config.RuntimeConfigFor(cfg),
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "kube-system",
			},
		},
	}

	rule := NamespaceRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestNamespaceRuleNotInAllowedList(t *testing.T) {
	assert := assert.New(t)

	cfg := &config.Config{
		AllowedNamespaces: []string{"kube-system"},
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

	rule := NamespaceRule{}
	result := suppresses(rule, ctx)
	assert.True(result)
}

func TestNamespaceRuleNoConfig(t *testing.T) {
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

	rule := NamespaceRule{}
	result := suppresses(rule, ctx)
	assert.False(result)
}
