package observe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
)

func TestPodOwnerUsesMarkedController(t *testing.T) {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "other", Kind: "Job"},
				{
					Name:       "deployment",
					Kind:       "Deployment",
					Controller: &controller,
				},
			},
		},
	}

	owner := PodOwners{}.OwnerOf(pod)

	assert.Equal(t, "Deployment", owner.Kind)
	assert.Equal(t, "deployment", owner.Name)
}

func TestPodOwnerRejectsAmbiguousOwners(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "one", Kind: "Job"},
				{Name: "two", Kind: "Job"},
			},
		},
	}

	assert.Equal(t, model.ObjectRef{}, PodOwners{}.OwnerOf(pod))
}

func TestPodOwnerUsesSingleUnmarkedOwner(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "job-1", Kind: "Job"},
			},
		},
	}

	assert.Equal(
		t,
		model.ObjectRef{Kind: "Job", Namespace: "ns", Name: "job-1"},
		PodOwners{}.OwnerOf(pod),
	)
}

func TestPodOwnerUsesMarkedParentController(t *testing.T) {
	controller := true
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	err := indexer.Add(&appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-1",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "other", Kind: "Job"},
				{
					Name:       "deployment",
					Kind:       "Deployment",
					Controller: &controller,
				},
			},
		},
	})
	assert.NoError(t, err)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "ns",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "rs-1", Kind: "ReplicaSet"},
			},
		},
	}
	owner := PodOwners{
		RS: appsv1lister.NewReplicaSetLister(indexer),
	}.OwnerOf(pod)

	assert.Equal(t, "Deployment", owner.Kind)
	assert.Equal(t, "deployment", owner.Name)
}
