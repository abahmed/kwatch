//go:build e2e

package scenarios

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodOOMKilled(t *testing.T) {
	inNamespace(t, "pod.oom-killed", func(s *Scenario) {
		s.CreatePod(podOverMemoryLimit("oom"))
		s.WaitForPodReason(5*time.Minute, 1, "OOMKilled")
		s.ExpectIncident("oom", "OOMKilled", 0)
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioPodUnschedulable(t *testing.T) {
	inNamespace(t, "pod.pending-unschedulable", func(s *Scenario) {
		s.CreatePod(podTooBigToSchedule("unschedulable"))
		s.ExpectRoot(harness.RootExpectation{
			Root:         "scheduling//Insufficient cpu*",
			Tier:         "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"pod/" + s.Namespace + "/unschedulable"},
		})
	})
}
