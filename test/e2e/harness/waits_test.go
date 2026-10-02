//go:build e2e

package harness

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

func int32Ptr(v int32) *int32 { return &v }

func TestDeploymentRolledOutRequiresCurrentGeneration(t *testing.T) {
	ready := func() *appsv1.Deployment {
		d := &appsv1.Deployment{}
		d.Generation = 2
		d.Spec.Replicas = int32Ptr(2)
		d.Status = appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           2,
			UpdatedReplicas:    2,
			AvailableReplicas:  2,
		}
		return d
	}
	tests := []struct {
		name   string
		mutate func(*appsv1.Deployment)
		want   bool
	}{
		{"complete rollout", func(*appsv1.Deployment) {}, true},
		{"stale generation", func(d *appsv1.Deployment) {
			d.Status.ObservedGeneration = 1
		}, false},
		{"old replicas remain", func(d *appsv1.Deployment) {
			d.Status.UpdatedReplicas = 1
		}, false},
		{"surge replica present", func(d *appsv1.Deployment) {
			d.Status.Replicas = 3
		}, false},
		{"unavailable replica", func(d *appsv1.Deployment) {
			d.Status.AvailableReplicas = 1
		}, false},
		{"scaled to zero", func(d *appsv1.Deployment) {
			d.Spec.Replicas = int32Ptr(0)
			d.Status.Replicas = 0
			d.Status.UpdatedReplicas = 0
			d.Status.AvailableReplicas = 0
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := ready()
			tc.mutate(d)
			require.Equal(t, tc.want, deploymentRolledOut(d))
		})
	}
}
