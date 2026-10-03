//go:build e2e

package scenarios

import "testing"

// All four pods run at once, so the scenario costs one settle time.
func TestScenarioPodFailureProfiles(t *testing.T) {
	inNamespace(t, "pod.failure-profiles", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsWorkload("startup-error", "startup-error"),
			podsDelayedError("delayed-error"),
			podsWorkload("one-shot-error", "one-shot-error"),
			podsWorkload("recurring-error", "recurring-error")))
		for _, name := range []string{"startup-error", "delayed-error",
			"one-shot-error", "recurring-error"} {
			s.ExpectIncident(name, "CrashLoopBackOff", 0)
		}
		s.ExpectKwatchHealthy()
	})
}
