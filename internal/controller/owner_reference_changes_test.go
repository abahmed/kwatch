package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodGraphInputsChangedWhenOwnerIsReplaced(t *testing.T) {
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Name: "old-owner", Kind: "ReplicaSet"},
			},
		},
	}
	newPod := old.DeepCopy()
	newPod.OwnerReferences[0].Name = "new-owner"

	assert.True(t, podGraphInputsChanged(old, newPod))
}

func TestPodGraphInputsIgnoreUnchangedOwners(t *testing.T) {
	controller := true
	old := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "owner",
					Kind:       "Deployment",
					Controller: &controller,
				},
			},
		},
	}
	newPod := old.DeepCopy()

	assert.False(t, podGraphInputsChanged(old, newPod))
}
