//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioExtendedTLS(t *testing.T) {
	inExtendedNamespace(t, "integration.tls", func(s *Scenario) {
		s.CreateExpiredTLSSecret("expired-tls")
		s.ExpectIncident("expired-tls", "TLSCertExpired", 0)
	})
}

func TestScenarioExtendedAdmissionWebhook(t *testing.T) {
	inExtendedNamespace(t, "security.admission-webhook", func(s *Scenario) {
		s.CreateWebhookWithoutService()
		s.ExpectRoot(harness.RootExpectation{
			Root: "validatingwebhookconfiguration//" +
				missingWebhookName,
			Tier:        "notify",
			MaxMessages: 2,
		})
	})
}

func TestScenarioExtendedMetricsAPIFailure(t *testing.T) {
	inExtendedNamespace(t, "integration.metrics-api", func(s *Scenario) {
		restoreMetricsAPI := s.BreakMetricsAPI()
		s.CreateDeployment("metrics-target", "healthy",
			withCPURequest("10m"))
		s.CreateAutoscaler(cpuAutoscaler("metrics-target"))
		s.ExpectClusterIncident(metricsAPIName,
			"FailedGetResourceMetric", detectors.DefaultConditionGrace)
		restoreMetricsAPI()
	})
}

func TestScenarioExtendedVolumeAttachment(t *testing.T) {
	inExtendedCluster(t, "storage.volume-attachment", func(s *Scenario) {
		name := s.CreateFailedVolumeAttachment()
		s.ExpectClusterIncident(name, "VolumeAttachmentFailure", 0)
	})
}
