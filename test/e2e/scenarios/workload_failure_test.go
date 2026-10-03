//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioDeploymentRolloutFailure(t *testing.T) {
	inNamespace(t, "workload.deployment-rollout", func(s *Scenario) {
		s.CreateDeployment("rollout", "healthy")
		s.WaitForRollout("rollout")

		s.BreakRollout("rollout")
		s.ExpectIncident("rollout", "ProgressDeadlineExceeded", 0)
		s.ExpectRoot(harness.RootExpectation{
			Root:        "deployment/" + s.Namespace + "/rollout",
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: append(s.ScheduledNodes(),
				"registry//example.invalid"),
		})
	})
}

// With no retries the Job fails with BackoffLimitExceeded.
func TestScenarioJobFailure(t *testing.T) {
	inNamespace(t, "workload.job", func(s *Scenario) {
		s.CreateJob(failingJob("failed-job"))
		s.ExpectIncident("failed-job", "JobBackoffLimitExceeded", 0)
	})
}
