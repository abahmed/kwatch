//go:build e2e

package scenarios

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodCrashLoop(t *testing.T) {
	inNamespace(t, "pod.crashloop", func(s *Scenario) {
		s.Must(createFailingDeployment(s.Ctx, s.Env, s.Namespace, "crashloop"))
		s.WaitForPods(5*time.Minute, func(pods []corev1.Pod) bool {
			return len(pods) == 1 &&
				harness.PodHasReason(&pods[0], "CrashLoopBackOff")
		})
		s.ExpectWorkloadRoot("deployment", "crashloop")
		s.ExpectKwatchHealthy()
	})
}
