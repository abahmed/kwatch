package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestSidecarStartingOnlyBeforeStartupProbePasses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sidecar bool
		started bool
		want    bool
	}{
		{"sidecar not started", true, false, true},
		{"sidecar started", true, true, false},
		{"classic init not started", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := pod("p1")
			init := corev1.Container{Name: "helper", Image: "helper:1"}
			if tc.sidecar {
				always := corev1.ContainerRestartPolicyAlways
				init.RestartPolicy = &always
			}
			p.Spec.InitContainers = []corev1.Container{init}
			p.Spec.Containers = nil
			started := tc.started
			p.Status.InitContainerStatuses = []corev1.ContainerStatus{{
				Name: "helper", Started: &started,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{}},
			}}
			descs := containerDescriptionsViaSchema(t, p)
			_, set := descs[0].Attributes[kube.AttrSidecarStarting]
			assert.Equal(t, tc.want, set)
		})
	}
}
