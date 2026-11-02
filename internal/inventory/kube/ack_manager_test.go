package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ackFields is the field set of a manager that wrote the ack annotation.
func ackFields() *metav1.FieldsV1 {
	return &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:annotations":` +
		`{".":{},"f:` + kube.AckAnnotation + `":{}}}}`)}
}

func ackedPod(entries ...metav1.ManagedFieldsEntry) *corev1.Pod {
	pod := maintenancePod(map[string]string{
		kube.AckAnnotation: "on it"})
	pod.ManagedFields = entries
	return pod
}

func TestAckByNamesTheWriterOfTheAnnotation(t *testing.T) {
	at := fixedTime()
	pod := ackedPod(
		updateEntry("kubectl-annotate", at, ackFields()),
		updateEntry("kubelet", at.Add(time.Minute), nil))

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, at))

	assert.Equal(t, "kubectl-annotate", attrs[kube.AttrAckBy].AsText())
}

// The informer transform drops the field sets and keeps only the latest
// writer, which is not the one that set the annotation.
func TestAckBySurvivesTheInformerTransform(t *testing.T) {
	at := fixedTime()
	pod := ackedPod(
		updateEntry("kubectl-annotate", at, ackFields()),
		updateEntry("kubelet", at.Add(time.Minute), nil))

	trimmed, err := kube.TrimToLatestManager(pod)
	require.NoError(t, err)

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(trimmed, true, at))
	assert.Equal(t, "kubectl-annotate", attrs[kube.AttrAckBy].AsText())
}

func TestAckByPrefersTheLatestWriter(t *testing.T) {
	at := fixedTime()
	pod := ackedPod(
		updateEntry("kubectl-client-side-apply", at, ackFields()),
		updateEntry("kubectl-annotate", at.Add(time.Hour), ackFields()))

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, at))

	assert.Equal(t, "kubectl-annotate", attrs[kube.AttrAckBy].AsText())
}

func TestAckByIsAbsentWithoutProof(t *testing.T) {
	at := fixedTime()
	pod := ackedPod(updateEntry("kubelet", at, nil))

	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(pod, true, at))

	assert.Contains(t, attrs, kube.AttrAck)
	assert.NotContains(t, attrs, kube.AttrAckBy)
}

// A stamp set on the cluster object is not trusted: the transform
// replaces it, and drops it from an object without the annotation.
func TestAckByIgnoresAPlantedStamp(t *testing.T) {
	pod := maintenancePod(map[string]string{
		"kwatch.abahmed.dev/ack-manager": "someone"})

	trimmed, err := kube.TrimToLatestManager(pod)
	require.NoError(t, err)

	cached := trimmed.(*corev1.Pod)
	assert.NotContains(t, cached.Annotations,
		"kwatch.abahmed.dev/ack-manager")
	attrs := podAttributes(kube.NewTranslator(kube.PodSchema{}).
		Added(cached, true, fixedTime()))
	assert.NotContains(t, attrs, kube.AttrAckBy)
}
