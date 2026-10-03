//go:build e2e

package scenarios

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioProviderFailureRecovery(t *testing.T) {
	inNamespace(t, "lifecycle.provider-failure", func(s *Scenario) {
		s.Must(s.Env.Receiver.Clear(s.Ctx))
		lifecycleReceiverMode(s, "http-500")
		lifecycleCreateCrashPod(s, "provider-failure")
		s.WaitForPods(5*time.Minute, func(pods []corev1.Pod) bool {
			return len(pods) == 1 &&
				harness.PodHasReason(&pods[0], "CrashLoopBackOff")
		})
		s.ExpectIncident("provider-failure", "CrashLoopBackOff", 0)

		requests, err := s.Env.Receiver.WaitForCount(s.Ctx, 1)
		s.Must(err)
		if requests[0].Status < 500 {
			t.Fatalf("expected provider failure, got HTTP %d",
				requests[0].Status)
		}
		lifecycleReceiverMode(s, "success")
		s.ExpectKwatchHealthy()
	})
}
