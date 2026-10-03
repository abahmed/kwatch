//go:build e2e

package scenarios

import (
	"testing"
	"time"
)

func TestScenarioPodLivenessFailure(t *testing.T) {
	inNamespace(t, "pod.liveness-failure", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsFailingLiveness("liveness")))
		// The liveness detector needs the failure to last two minutes.
		s.ExpectIncident("liveness", "LivenessProbeFailed",
			2*time.Minute)
		s.ExpectKwatchHealthy()
	})
}
