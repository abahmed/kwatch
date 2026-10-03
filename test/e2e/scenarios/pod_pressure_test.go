//go:build e2e

package scenarios

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestScenarioPodEphemeralStorageEviction(t *testing.T) {
	inNamespace(t, "pod.ephemeral-storage-pressure", func(s *Scenario) {
		s.CreatePod(podWithLimit(workloadPod("disk-pressure", "disk"),
			corev1.ResourceEphemeralStorage, "1Mi"))
		s.ExpectIncident("disk-pressure", "Evicted", 0)
	})
}
