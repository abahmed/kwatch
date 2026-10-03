//go:build e2e

package scenarios

import "testing"

func TestScenarioPodLivenessFailure(t *testing.T) {
	inNamespace(t, "pod.liveness-failure", func(s *Scenario) {
		s.CreatePod(podWithFailingLiveness("liveness"))
		// Failed liveness probes restart the container, so Kwatch reports
		// the restarts as a crash loop.
		s.ExpectIncident("liveness", "CrashLoopBackOff", 0)
		s.ExpectKwatchHealthy()
	})
}
