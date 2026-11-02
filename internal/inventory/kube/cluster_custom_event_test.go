package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An event about a cluster-scoped custom resource must land on the same
// entity the resource is described as, or recovery could never apply.
func TestEventNoteLandsOnClusterScopedCustomResource(t *testing.T) {
	policy := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kyverno.io/v1", "kind": "ClusterPolicy",
		"metadata": map[string]any{"name": "require-labels"},
	}}
	described, ok := kube.NewUnstructuredSchema(
		"kyverno.io", "ClusterPolicy").Describe(policy)
	require.True(t, ok)

	note, ok := kube.EventNote(&corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "kyverno.io/v1", Kind: "ClusterPolicy",
			Name: "require-labels",
		},
		Type: corev1.EventTypeWarning, Reason: "PolicyError",
	}, fixedTime())
	require.True(t, ok)

	assert.Equal(t, described.ID, note.Entity)
}
