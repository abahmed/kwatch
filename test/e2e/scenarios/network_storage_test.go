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
		s.ExpectGatheredEvidence(s.Namespace, "empty-service")
	})
}

func TestScenarioPersistentVolumeClaimFailure(t *testing.T) {
	inNamespace(t, "storage.pvc-pv", func(s *Scenario) {
		s.CreateVolumeClaim(claimWithMissingStorageClass("missing-volume"))
		// The claim names a StorageClass that does not exist, so the
		// incident is rooted at that StorageClass.
		s.ExpectClusterIncident(missingStorageClass, "ProvisioningFailed",
			detectors.DefaultClaimPending)
		s.ExpectRoot(harness.RootExpectation{
			Root:        "storageclass//" + missingStorageClass,
			Tier:        "notify",
			MaxMessages: 2,
		})
		s.ExpectGatheredEvidence("", missingStorageClass)
	})
}
