package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodOwnersFilterAlreadySet(t *testing.T) {
	assert := assert.New(t)

	owner := metav1.OwnerReference{
		Name: "existing-owner",
		Kind: "Deployment",
	}

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Owner: &owner,
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Equal("existing-owner", ctx.Owner.Name)
}

func TestPodOwnersFilterNoOwnerReferences(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
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

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Nil(ctx.Owner)
}

func TestPodOwnersFilterDirectOwner(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
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

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.NotNil(ctx.Owner)
	assert.Equal("direct-deployment", ctx.Owner.Name)
}

func TestPodOwnersFilterReplicaSet(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
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

	filter := PodOwnersFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.Nil(
		ctx.Owner,
		"owner should remain nil when ReplicaSet API lookup fails",
	)
}

func TestPodStatusFilterSucceeded(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}

func TestPodStatusFilterAddedWithNoConditions(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		EvType: "Added",
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusFilterPodCompleted(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionFalse,
						Reason: "PodCompleted",
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.True(result)
	assert.False(ctx.PodHasIssues)
}

func TestPodStatusFilterPodReady(t *testing.T) {
	assert := assert.New(t)

	ctx := &Context{
		Sources: Sources{
			Client: fake.NewSimpleClientset(),
			Config: &config.Config{},
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}

	filter := PodStatusFilter{}
	result := filter.Execute(ctx)
	assert.False(result)
	assert.False(ctx.PodHasIssues)
	assert.False(ctx.ContainersHasIssues)
}
