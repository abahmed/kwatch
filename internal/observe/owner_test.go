package observe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

func TestPodOwnerOfNoOwner(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
	}
	name := PodOwners{}.OwnerOf(pod).Name
	assert.Equal(t, "p1", name)
}

func TestPodOwnerOfReplicaSet(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "dep1", Kind: "Deployment"},
			},
		},
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	err := indexer.Add(rs)
	assert.NoError(t, err)
	rsLister := appsv1lister.NewReplicaSetLister(indexer)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "rs1", Kind: "ReplicaSet"},
			},
		},
	}
	name := PodOwners{RS: rsLister}.OwnerOf(pod).Name
	assert.Equal(t, "dep1", name)
}

func TestPodOwnerOfReplicaSetNoGrandparent(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rs1", Namespace: "ns1"},
	}
	err := indexer.Add(rs)
	assert.NoError(t, err)
	rsLister := appsv1lister.NewReplicaSetLister(indexer)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "rs1", Kind: "ReplicaSet"},
			},
		},
	}
	name := PodOwners{RS: rsLister}.OwnerOf(pod).Name
	assert.Equal(t, "rs1", name)
}

func TestPodOwnerOfReplicaSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "rs1", Kind: "ReplicaSet"},
			},
		},
	}
	name := PodOwners{}.OwnerOf(pod).Name
	assert.Empty(t, name)
}

func TestPodOwnerOfDaemonSet(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ds1", Namespace: "ns1"},
	}
	err := indexer.Add(ds)
	assert.NoError(t, err)
	dsLister := appsv1lister.NewDaemonSetLister(indexer)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ds1", Kind: "DaemonSet"},
			},
		},
	}
	name := PodOwners{DS: dsLister}.OwnerOf(pod).Name
	assert.Equal(t, "ds1", name)
}

func TestPodOwnerOfDaemonSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ds1", Kind: "DaemonSet"},
			},
		},
	}
	name := PodOwners{}.OwnerOf(pod).Name
	assert.Empty(t, name)
}

func TestPodOwnerOfStatefulSet(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ss1", Namespace: "ns1"},
	}
	err := indexer.Add(ss)
	assert.NoError(t, err)
	ssLister := appsv1lister.NewStatefulSetLister(indexer)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ss1", Kind: "StatefulSet"},
			},
		},
	}
	name := PodOwners{SS: ssLister}.OwnerOf(pod).Name
	assert.Equal(t, "ss1", name)
}

func TestPodOwnerOfStatefulSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ss1", Kind: "StatefulSet"},
			},
		},
	}
	name := PodOwners{}.OwnerOf(pod).Name
	assert.Empty(t, name)
}

func TestPodOwnerOfUnknownKind(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "custom1", Kind: "CustomResource"},
			},
		},
	}
	name := PodOwners{}.OwnerOf(pod).Name
	assert.Equal(t, "custom1", name)
}
