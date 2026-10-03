//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioServiceWithoutEndpoints(t *testing.T) {
	inNamespace(t, "networking.service-endpoint", func(s *Scenario) {
		s.CreateService(emptyService("empty-service"))

		// A Service that was never backed is empty on purpose. One that
		// emptied after its selector changed was broken by the edit.
		s.ChangeServiceSelector("empty-service", "renamed")
		s.ExpectIncident("empty-service", "ServiceNoEndpoints",
			detectors.DefaultNoEndpoints)
	})
}

func TestScenarioPersistentVolumeClaimFailure(t *testing.T) {
	inNamespace(t, "storage.pvc-pv", func(s *Scenario) {
		s.CreateVolumeClaim(claimWithMissingStorageClass("missing-volume"))
		s.ExpectIncident("missing-volume", "PersistentVolumeClaimFailure",
			detectors.DefaultClaimPending)
		s.ExpectRoot(harness.RootExpectation{
			Root: "persistentvolumeclaim/" + s.Namespace +
				"/missing-volume",
			Tier:        "notify",
			MaxMessages: 2,
		})
	})
}
