package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodOwnersEnricherStatefulSet(t *testing.T) {
	assert := assert.New(t)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-sts",
			Namespace: "default",
		},
	}
	client := fake.NewSimpleClientset(sts)

	ctx := &Context{
		Sources: Sources{
			Client:  client,
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Owner: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-sts-0",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "my-sts",
						Kind: "StatefulSet",
					},
				},
			},
		},
	}

	enricher := PodOwnersEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
	assert.Equal("my-sts", ctx.Owner.Name)
	assert.Equal("StatefulSet", ctx.Owner.Kind)
}

func TestPodOwnersEnricherAlreadySet(t *testing.T) {
	assert := assert.New(t)

	owner := metav1.OwnerReference{
		Name: "existing-owner",
		Kind: "Deployment",
	}

	ctx := &Context{
		Sources: Sources{
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Owner: &owner,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	enricher := PodOwnersEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.Equal("existing-owner", ctx.Owner.Name)
}

func TestPodOwnersEnricherNoOwnerReferences(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Owner: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-pod",
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{},
			},
		},
	}

	enricher := PodOwnersEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.Nil(ctx.Owner)
}

func TestPodOwnersEnricherDirectOwner(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Owner: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "direct-deployment",
						Kind: "Deployment",
					},
				},
			},
		},
	}

	enricher := PodOwnersEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
	assert.Equal("direct-deployment", ctx.Owner.Name)
}

func TestPodOwnersEnricherReplicaSet(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client:  fake.NewSimpleClientset(),
			Runtime: config.RuntimeConfigFor(&config.Config{}),
		},
		Owner: nil,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Name: "my-rs",
						Kind: "ReplicaSet",
					},
				},
			},
		},
	}

	enricher := PodOwnersEnricher{}
	result := enricher.Enrich(ctx)
	assert.False(result)
	assert.Nil(
		ctx.Owner,
		"owner should remain nil when ReplicaSet API lookup fails",
	)
}
