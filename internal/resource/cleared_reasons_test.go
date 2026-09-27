package resource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func TestClearedReasonsOmitsActiveLevelKeepsOthers(t *testing.T) {
	nodeIndex := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	for _, name := range []string{"n1", "n2"} {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		if err := nodeIndex.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	m := &Monitor{
		nodeLister: corev1listers.NewNodeLister(nodeIndex),
	}

	obs := observe.NodeNamed("n1", constant.ReasonNodeResourceHigh)
	cleared := m.clearedReasons([]*model.Observation{obs}, nil)

	assertContains := func(node, reason string) {
		for _, c := range cleared {
			if c.node == node && c.reason == reason {
				return
			}
		}
		t.Fatalf("expected cleared (%s, %s) in %+v", node, reason, cleared)
	}
	assertAbsent := func(node, reason string) {
		for _, c := range cleared {
			if c.node == node && c.reason == reason {
				t.Fatalf("unexpected cleared (%s, %s) in %+v",
					node, reason, cleared)
			}
		}
	}

	assertContains("n1", constant.ReasonNodeResourceCritical)
	assertAbsent("n1", constant.ReasonNodeResourceHigh)
	assertContains("n2", constant.ReasonNodeResourceHigh)
	assertContains("n2", constant.ReasonNodeResourceCritical)
}

func TestClearedReasonsFilesystemOnlyForFsNodes(t *testing.T) {
	nodeIndex := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	for _, name := range []string{"n1", "n2"} {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		if err := nodeIndex.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	m := &Monitor{
		nodeLister: corev1listers.NewNodeLister(nodeIndex),
	}

	cleared := m.clearedReasons(nil, []string{"n1"})

	hasFS := func(node string) bool {
		for _, c := range cleared {
			if c.node == node &&
				c.reason == constant.ReasonNodeFilesystemHigh {
				return true
			}
		}
		return false
	}

	if !hasFS("n1") {
		t.Fatalf("expected filesystem reason cleared for n1: %+v", cleared)
	}
	if hasFS("n2") {
		t.Fatalf("did not expect filesystem reason cleared for n2: %+v",
			cleared)
	}
}
