//go:build e2e

package scenarios

import "testing"

// All four pods run at once, so the scenario costs one settle time.
func TestScenarioPodFailureProfiles(t *testing.T) {
	inNamespace(t, "pod.failure-profiles", func(s *Scenario) {
		s.CreatePod(workloadPod("startup-error", "startup-error"))
		s.CreatePod(podWithDelayedError("delayed-error"))
		s.CreatePod(workloadPod("one-shot-error", "one-shot-error"))
		s.CreatePod(workloadPod("recurring-error", "recurring-error"))
		for _, name := range []string{"startup-error", "delayed-error",
			"one-shot-error", "recurring-error"} {
			s.ExpectIncident(name, "CrashLoopBackOff", 0)
		}
		s.ExpectKwatchHealthy()
	})
}
