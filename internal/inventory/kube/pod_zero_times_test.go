package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A status that leaves a time unset (a condition or a last termination
// without its timestamp) records no time: a zero time would read as an
// event in year one, older than every window.
func TestUnsetStatusTimesAreNotRecorded(t *testing.T) {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "c"}}},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "c",
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1, Reason: "Error"}},
			}},
		},
	}
	desc, ok := kube.PodSchema{}.Describe(p)
	assert.True(t, ok)
	assert.NotContains(t, desc.Attributes, kube.AttrReadySince)
	children := containerDescriptionsViaSchema(t, p)
	assert.Len(t, children, 1)
	assert.Contains(t, children[0].Attributes, kube.AttrLastExitCode)
	assert.NotContains(t, children[0].Attributes, kube.AttrLastFinished)
}
