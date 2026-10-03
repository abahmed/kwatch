//go:build e2e

package scenarios

import (
	"testing"
	"time"
)

func TestScenarioActiveProbeFailureAndRecovery(t *testing.T) {
	onCluster(t, "integration.active-probe", func(s *Scenario) {
		s.SetReceiverMode("http-500")
		s.CreateKwatchConfig("kwatch-e2e-active-probe", activeProbeSpec())
		incident := s.ExpectClusterIncident("receiver", "ActiveProbeFailure",
			2*time.Minute)

		s.SetReceiverMode("success")
		s.ExpectResolved(incident)
	})
}

func TestScenarioHeartbeatDelivery(t *testing.T) {
	onCluster(t, "integration.heartbeat", func(s *Scenario) {
		s.ClearReceiver()
		s.StartHeartbeatKwatch()
		s.ExpectHeartbeat()
	})
}
