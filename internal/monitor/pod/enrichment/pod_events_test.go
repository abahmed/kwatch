package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

func TestPodEventsEnricherNilEvents(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Findings: policy.Findings{
			PodHasIssues: true,
		},
		Events: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	enricher := PodEventsEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}

func TestPodEventsEnricherWarningDeletingPod(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Findings: policy.Findings{
			PodHasIssues: true,
		},
		Events: &[]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Message: "deleting pod",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	enricher := PodEventsEnricher{}
	result := enricher.Enrich(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestPodEventsEnricherNotPodHasIssues(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Findings: policy.Findings{
			PodHasIssues: false,
		},
		Events: &[]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Message: "deleting pod",
			},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	enricher := PodEventsEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
}
