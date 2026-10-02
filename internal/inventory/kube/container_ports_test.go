package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestContainerPortAttributes: declared ports and probe ports are
// recorded; named probe ports resolve to the container's port of that
// name, and an unknown name is kept as written.
func TestContainerPortAttributes(t *testing.T) {
	p := pod("p1")
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080},
			{Name: "metrics", ContainerPort: 9090},
		},
		ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Port: intstr.FromString("http")},
		}},
		LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromString("admin")},
		}},
		StartupProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{Port: intstr.FromInt32(8081)},
		}},
	}}
	containers := containerDescriptionsViaSchema(t, p)
	require.Len(t, containers, 1)
	attrs := containers[0].Attributes
	assert.Equal(t, "8080,9090", attrs[kube.AttrContainerPorts].AsText())
	assert.Equal(t, "8081,8080,admin", attrs[kube.AttrProbePorts].AsText())
}

// TestContainerLastRunStart: the start of the last terminated run is
// recorded next to its finish.
func TestContainerLastRunStart(t *testing.T) {
	p := pod("p1")
	p.Spec.Containers = []corev1.Container{{Name: "app", Image: "app:1"}}
	started := metav1.NewTime(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app",
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason: "OOMKilled", ExitCode: 137, StartedAt: started,
				FinishedAt: metav1.NewTime(started.Add(40 * time.Second)),
			},
		},
	}}
	attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
	assert.True(t, attrs[kube.AttrLastStarted].AsTime().Equal(started.Time))
}
