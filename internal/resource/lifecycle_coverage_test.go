package resource

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
)

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	nodeIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	podIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	monitor := NewMonitor(Config{Interval: time.Millisecond},
		corev1listers.NewNodeLister(nodeIndex),
		corev1listers.NewPodLister(podIndex))
	monitor.Run(ctx, func(*model.Observation) {
		t.Fatal("canceled monitor invoked callback")
	})
}

func TestCheckSkipsNodesWithoutAllocatableResources(t *testing.T) {
	nodeIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	podIndex := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := nodeIndex.Add(&corev1.Node{}); err != nil {
		t.Fatal(err)
	}
	monitor := NewMonitor(Config{},
		corev1listers.NewNodeLister(nodeIndex),
		corev1listers.NewPodLister(podIndex))
	if got := monitor.Check(); len(got) != 0 {
		t.Fatalf("signals = %v", got)
	}
}
