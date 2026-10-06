package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTypedTransformDropsTheLastAppliedCopy(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "p", Namespace: "ns",
		Annotations: map[string]string{
			lastAppliedAnnotation: `{"a full":"copy of the object"}`,
			"team":                "payments",
		},
	}}

	out, err := testTransform(pod)

	require.NoError(t, err)
	got := out.(*corev1.Pod).Annotations
	assert.NotContains(t, got, lastAppliedAnnotation)
	assert.Equal(t, "payments", got["team"], "other annotations stay")
}

func TestTypedTransformKeepsObjectsWithoutAnnotations(t *testing.T) {
	out, err := testTransform(&corev1.Pod{})
	require.NoError(t, err)
	assert.Empty(t, out.(*corev1.Pod).Annotations)
}
