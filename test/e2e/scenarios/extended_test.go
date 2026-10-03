//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioExtendedTLS(t *testing.T) {
	inExtendedNamespace(t, "integration.tls", func(s *Scenario) {
		extCreateExpiredTLSSecret(s, "expired-tls")
		s.ExpectIncident("expired-tls", "TLSCertExpired", 0)
	})
}

func TestScenarioExtendedAdmissionWebhook(t *testing.T) {
	inExtendedNamespace(t, "security.admission-webhook", func(s *Scenario) {
		extCreateWebhookWithoutService(s)
		s.ExpectRoot(harness.RootExpectation{
			Root:        "validatingwebhookconfiguration//" + extWebhookName,
			Tier:        "notify",
			MaxMessages: 2,
		})
	})
}

func TestScenarioExtendedMetricsAPIFailure(t *testing.T) {
	inExtendedNamespace(t, "integration.metrics-api", func(s *Scenario) {
		restoreMetricsAPI := extBreakMetricsAPI(s)
		extCreateAutoscaledDeployment(s)
		s.ExpectClusterIncident(extMetricsAPIName,
			"FailedGetResourceMetric", detectors.DefaultConditionGrace)
		restoreMetricsAPI()
	})
}

func TestScenarioExtendedVolumeAttachment(t *testing.T) {
	inExtendedCluster(t, "storage.volume-attachment", func(s *Scenario) {
		name := extCreateFailedVolumeAttachment(s)
		s.ExpectClusterIncident(name, "VolumeAttachmentFailure", 0)
	})
}
