//go:build e2e

package scenarios

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioDeploymentRolloutFailure(t *testing.T) {
	inNamespace(t, "workload.deployment-rollout", func(s *Scenario) {
		_, err := createLifecycleDeployment(
			s.Ctx, s.Env, s.Namespace, "rollout", "healthy")
		s.Must(err)
		s.Must(s.Env.WaitForDeployment(s.Ctx, s.Namespace, "rollout"))

		s.Must(workloadsBreakRollout(s.Ctx, s.Env, s.Namespace, "rollout"))
		s.ExpectIncident("rollout", "ProgressDeadlineExceeded", 0)
		s.ExpectRoot(harness.RootExpectation{
			Root:        "deployment/" + s.Namespace + "/rollout",
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: append(
				scheduledNodes(s.Ctx, s.T, s.Env, s.Namespace),
				"registry//example.invalid"),
		})
	})
}

// With no retries the Job fails with BackoffLimitExceeded.
func TestScenarioJobFailure(t *testing.T) {
	inNamespace(t, "workload.job", func(s *Scenario) {
		_, err := s.Env.Client.BatchV1().Jobs(s.Namespace).Create(
			s.Ctx, workloadsFailingJob("failed-job"),
			metav1.CreateOptions{})
		s.Must(err)
		s.ExpectIncident("failed-job", "JobBackoffLimitExceeded", 0)
	})
}
