//go:build e2e

package scenarios

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioPodOOMKilled(t *testing.T) {
	inNamespace(t, "pod.oom-killed", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsOverMemory("oom")))
		s.WaitForPods(5*time.Minute, func(pods []corev1.Pod) bool {
			return len(pods) == 1 &&
				harness.PodHasReason(&pods[0], "OOMKilled")
		})
		s.ExpectIncident("oom", "OOMKilled", 0)
		s.ExpectKwatchHealthy()
	})
}

func TestScenarioPodUnschedulable(t *testing.T) {
	inNamespace(t, "pod.pending-unschedulable", func(s *Scenario) {
		s.Must(podsCreate(s.Ctx, s.Env, s.Namespace,
			podsTooBig("unschedulable")))
		s.ExpectRoot(harness.RootExpectation{
			Root:         "scheduling//Insufficient cpu*",
			Tier:         "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"pod/" + s.Namespace + "/unschedulable"},
		})
	})
}
