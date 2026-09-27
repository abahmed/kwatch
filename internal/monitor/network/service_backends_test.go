package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
)

func TestServiceBackendEvidenceIdentifiesSharedFailingNode(t *testing.T) {
	ready := corev1.ConditionFalse
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
	require.NoError(t, indexer.Add(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: ready,
			Reason: "KubeletNotReady",
		}}},
	}))
	finding := &model.Observation{}
	enrichServiceEndpointFinding(finding, []*corev1.Pod{
		backendPod("one", "node-a", false),
		backendPod("two", "node-a", false),
	}, corev1lister.NewNodeLister(indexer))
	assert.Equal(t, 2, finding.Facts.BackendPods)
	assert.Equal(t, 2, finding.Facts.UnreadyBackendPods)
	assert.Equal(t, "node-a", finding.Facts.SharedFailingNode)
	assert.Equal(t, "KubeletNotReady", finding.Facts.NodeFailureReason)
}

func TestServiceBackendEvidenceDoesNotInferNodeFailure(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
	require.NoError(t, indexer.Add(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue,
		}}},
	}))
	tests := []struct {
		name string
		pods []*corev1.Pod
	}{
		{"healthy node", []*corev1.Pod{
			backendPod("one", "node-a", false),
			backendPod("two", "node-a", false),
		}},
		{"different nodes", []*corev1.Pod{
			backendPod("one", "node-a", false),
			backendPod("two", "node-b", false),
		}},
		{"some pods ready", []*corev1.Pod{
			backendPod("one", "node-a", false),
			backendPod("two", "node-a", true),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finding := &model.Observation{}
			enrichServiceEndpointFinding(
				finding, test.pods,
				corev1lister.NewNodeLister(indexer),
			)
			assert.Empty(t, finding.Facts.SharedFailingNode)
		})
	}
}

func TestServiceBackendEvidenceLinksPartialFailureToNode(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
	require.NoError(t, indexer.Add(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionFalse,
			Reason: "KubeletNotReady",
		}}},
	}))
	finding := &model.Observation{}
	enrichServiceEndpointFinding(finding, []*corev1.Pod{
		backendPod("one", "node-a", false),
		backendPod("two", "node-b", true),
	}, corev1lister.NewNodeLister(indexer))
	assert.Equal(t, 1, finding.Facts.UnreadyBackendPods)
	assert.Equal(t, "node-a", finding.Facts.SharedFailingNode)
}

func backendPod(name, node string, ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type: corev1.PodReady, Status: status,
		}}},
	}
}
