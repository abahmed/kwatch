package correlation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

var ctx = context.Background()

func TestResolveOwnerNameNoOwner(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
	}
	name := ResolveOwnerName(pod, nil, nil, nil)
	assert.Equal(t, "p1", name)
}

func TestResolveOwnerNameReplicaSet(t *testing.T) {
	client := fake.NewSimpleClientset()
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "dep1", Kind: "Deployment"},
			},
		},
	}
	_, err := client.AppsV1().ReplicaSets("ns1").Create(ctx, rs, metav1.CreateOptions{})
	assert.NoError(t, err)

	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	err = indexer.Add(rs)
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
	name := ResolveOwnerName(pod, rsLister, nil, nil)
	assert.Equal(t, "dep1", name)
}

func TestResolveOwnerNameReplicaSetNoGrandparent(t *testing.T) {
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
	name := ResolveOwnerName(pod, rsLister, nil, nil)
	assert.Equal(t, "rs1", name)
}

func TestResolveOwnerNameReplicaSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "rs1", Kind: "ReplicaSet"},
			},
		},
	}
	name := ResolveOwnerName(pod, nil, nil, nil)
	assert.Empty(t, name)
}

func TestResolveOwnerNameDaemonSet(t *testing.T) {
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
	name := ResolveOwnerName(pod, nil, dsLister, nil)
	assert.Equal(t, "ds1", name)
}

func TestResolveOwnerNameDaemonSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ds1", Kind: "DaemonSet"},
			},
		},
	}
	name := ResolveOwnerName(pod, nil, nil, nil)
	assert.Empty(t, name)
}

func TestResolveOwnerNameStatefulSet(t *testing.T) {
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
	name := ResolveOwnerName(pod, nil, nil, ssLister)
	assert.Equal(t, "ss1", name)
}

func TestResolveOwnerNameStatefulSetNilLister(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "ss1", Kind: "StatefulSet"},
			},
		},
	}
	name := ResolveOwnerName(pod, nil, nil, nil)
	assert.Empty(t, name)
}

func TestResolveOwnerNameUnknownKind(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Name: "custom1", Kind: "CustomResource"},
			},
		},
	}
	name := ResolveOwnerName(pod, nil, nil, nil)
	assert.Equal(t, "custom1", name)
}
