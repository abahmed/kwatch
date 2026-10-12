//go:build e2e

package scenarios

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

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
	inExtendedNamespaceAlone(t, "security.admission-webhook", func(s *Scenario) {
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
	inExtendedNamespaceAlone(t, "integration.metrics-api", func(s *Scenario) {
		restoreMetricsAPI := s.BreakMetricsAPI()
		s.CreateDeployment("metrics-target", "healthy",
			withCPURequest("10m"))
		s.CreateAutoscaler(cpuAutoscaler("metrics-target"))
		// One autoscaler failing to read metrics waits ten minutes before
		// it is reported, so the incident is the unavailable API itself.
		s.ExpectClusterIncident(metricsAPIName,
			"APIServiceFailure", detectors.DefaultCustomFailing)
		restoreMetricsAPI()
	})
}

func TestScenarioExtendedVolumeAttachment(t *testing.T) {
	inExtendedCluster(t, "storage.volume-attachment", func(s *Scenario) {
		name := s.CreateFailedVolumeAttachment()
		// An attach error must last DefaultCustomFailing before it is
		// reported, because a successful retry clears it.
		s.ExpectClusterIncident(name, "VolumeAttachmentFailure",
			detectors.DefaultCustomFailing)
	})
}

// A policy that denies a workload's Pods leaves the ReplicaSet unable to
// create them. Pods without the policy's label in the same namespace are
// untouched, and lifting the policy lets the ReplicaSet recover.
func TestScenarioAdmissionPolicy(t *testing.T) {
	inExtendedNamespace(t, "security.admission-policy", func(s *Scenario) {
		lift := s.DenyLabeledPods()

		s.CreateReplicaSet(healthyReplicaSet("allowed"))
		s.WaitForPods(2*time.Minute, func(pods []corev1.Pod) bool {
			return podsOwnedBy(pods, "allowed")
		})

		s.CreateReplicaSet(deniedReplicaSet("denied"))
		incident := s.ExpectIncident("denied", "ReplicaSetFailure", 0)
		s.ExpectNoIncident("allowed")

		lift()
		s.WaitForPods(2*time.Minute, func(pods []corev1.Pod) bool {
			return podsOwnedBy(pods, "denied")
		})
		s.ExpectResolved(incident)
	})
}
