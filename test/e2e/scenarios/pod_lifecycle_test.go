//go:build e2e

package scenarios

import (
	"testing"
	"time"
)

func TestScenarioPodCrashLoop(t *testing.T) {
	inNamespace(t, "pod.crashloop", func(s *Scenario) {
		s.CreateDeployment("crashloop", "crash")
		s.WaitForPodReason(5*time.Minute, 1, "CrashLoopBackOff")
		s.ExpectWorkloadRoot("deployment", "crashloop")
		s.ExpectKwatchHealthy()
	})
}
