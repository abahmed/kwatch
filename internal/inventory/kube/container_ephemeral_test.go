package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A container added to debug a running pod is modelled on its own, and
// the warnings about it land on it: a debug image that will not pull
// is not the pod's trouble. It is not PartOf the pod, so nothing that
// judges the workload counts it.
func TestEphemeralContainerIsItsOwnEntity(t *testing.T) {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: testNamespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
			EphemeralContainers: []corev1.EphemeralContainer{{
				EphemeralContainerCommon: corev1.EphemeralContainerCommon{
					Name: "debugger", Image: "busybox"}}},
		},
		Status: corev1.PodStatus{EphemeralContainerStatuses: []corev1.
			ContainerStatus{{Name: "debugger", State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "ImagePullBackOff"}}}}},
	}
	id := kube.ContainerID(testNamespace, "p", "debugger")
	var debugger *kube.Description
	for _, c := range containerDescriptionsViaSchema(t, p) {
		if c.ID == id {
			debugger = &c
		}
	}
	require.NotNil(t, debugger)
	assert.Equal(t, inventory.Bool(true),
		debugger.Attributes[kube.AttrEphemeral])
	assert.Equal(t, "waiting", debugger.Attributes[kube.AttrState].AsText())
	assert.Empty(t, debugger.Relations)

	ev := warningEvent("Pod", "spec.ephemeralContainers{debugger}")
	observation, ok := kube.EventNote(ev, fixedTime())
	assert.True(t, ok)
	assert.Equal(t, id, observation.Entity)
}
