//go:build e2e

package scenarios

import "testing"

func TestScenarioPodLivenessFailure(t *testing.T) {
	inNamespace(t, "pod.liveness-failure", func(s *Scenario) {
		s.CreatePod(podWithFailingLiveness("liveness"))
		// Failed liveness probes keep killing the container, so Kwatch
		// reports a crash loop caused by the probe: LivenessKilled.
		s.ExpectIncident("liveness", "LivenessKilled", 0)
		s.ExpectKwatchHealthy()
	})
}
