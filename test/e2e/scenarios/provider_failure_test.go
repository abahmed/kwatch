//go:build e2e

package scenarios

import (
	"testing"
	"time"
)

func TestScenarioProviderFailureRecovery(t *testing.T) {
	inNamespaceAlone(t, "lifecycle.provider-failure", func(s *Scenario) {
		s.ClearReceiver()
		s.SetReceiverMode("http-500")
		s.CreatePod(crashingPod("provider-failure"))
		s.WaitForPodReason(5*time.Minute, 1, "CrashLoopBackOff")
		s.ExpectIncident("provider-failure", "CrashLoopBackOff", 0)

		s.ExpectDeliveryRejected()
		s.SetReceiverMode("success")
		s.ExpectKwatchHealthy()
	})
}
