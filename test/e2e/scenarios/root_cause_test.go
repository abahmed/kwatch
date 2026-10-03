//go:build e2e

package scenarios

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// TestScenarioRootCauseSharedNode stops one node under many Pods and
// expects a single node-rooted page instead of one message per Pod.
func TestScenarioRootCauseSharedNode(t *testing.T) {
	inNamespace(t, "rootcause.shared-node", func(s *Scenario) {
		nodeRequireKind(s)
		node := nodeToStop(s)
		blamed := nodeCreateSleepingDeployments(s, node, 6)

		nodeStopUntilNotReady(s, node)
		s.ExpectRoot(harness.RootExpectation{
			Root:             "node//" + node,
			Tier:             "page",
			MaxMessages:      2,
			MaxTotalMessages: 3,
			MustNotBlame:     blamed,
		})

		nodeStartUntilReady(s, node)
		s.ExpectKwatchHealthy()
	})
}

// TestScenarioRootCauseSharedRegistry points many workloads at one
// unreachable registry and expects the registry as the single root.
func TestScenarioRootCauseSharedRegistry(t *testing.T) {
	inNamespace(t, "rootcause.shared-registry", func(s *Scenario) {
		const registry = "registry.kwatch-e2e.invalid"
		var blamed []string
		for index := 0; index < 4; index++ {
			name := fmt.Sprintf("service-%d", index)
			blamed = append(blamed, "deployment/"+s.Namespace+"/"+name)
			s.Must(createDeployment(s.Ctx, s.Env, s.Namespace, name, 1,
				corev1.Container{
					Name:    "workload",
					Image:   registry + "/team/" + name + ":1",
					Command: []string{"sleep"},
				}, nil))
		}
		s.WaitForPods(8*time.Minute, func(pods []corev1.Pod) bool {
			return len(pods) == len(blamed) && allPodsHaveReason(
				pods, "ErrImagePull", "ImagePullBackOff")
		})

		s.ExpectRoot(harness.RootExpectation{
			Root:             "registry//" + registry,
			Tier:             "notify",
			MaxMessages:      2,
			MaxTotalMessages: 3,
			MustNotBlame:     blamed,
		})
	})
}

// TestScenarioRootCauseSmallStorm crashes 50 Pods of one workload and
// holds the whole namespace to three messages.
func TestScenarioRootCauseSmallStorm(t *testing.T) {
	inNamespace(t, "rootcause.small-storm", func(s *Scenario) {
		s.Must(createFailingDeploymentReplicas(
			s.Ctx, s.Env, s.Namespace, "storm", stormReplicas))
		s.WaitForPods(8*time.Minute, func(pods []corev1.Pod) bool {
			return len(pods) == stormReplicas &&
				allPodsHaveReason(pods, "CrashLoopBackOff")
		})

		s.ExpectRoot(harness.RootExpectation{
			Root:             "deployment/" + s.Namespace + "/storm",
			Tier:             "notify",
			MaxMessages:      3,
			MaxTotalMessages: 3,
			MustNotBlame: scheduledNodes(
				s.Ctx, s.T, s.Env, s.Namespace),
		})
	})
}
