//go:build e2e

package scenarios

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// stormReplicas is how many Pods the small-storm scenario crashes.
const stormReplicas = 50

// TestScenarioRootCauseSharedNode stops one node under many Pods and
// expects a single node-rooted page instead of one message per Pod.
func TestScenarioRootCauseSharedNode(t *testing.T) {
	inNamespaceAlone(t, "rootcause.shared-node", func(s *Scenario) {
		s.RequireKind()
		node := s.FreeWorkerNode()
		blamed := s.CreateSleepingDeployments(node, 6)

		s.StopNode(node)
		s.ExpectRoot(harness.RootExpectation{
			Root:             "node//" + node,
			Tier:             "page",
			MaxMessages:      2,
			MaxTotalMessages: 3,
			MustNotBlame:     blamed,
		})

		s.StartNode(node)
		s.ExpectKwatchHealthy()
	})
}

// TestScenarioRootCauseSharedRegistry points many workloads at one
// unreachable registry and expects the registry as the single root.
func TestScenarioRootCauseSharedRegistry(t *testing.T) {
	knownGap(t, "image pull failures of several Deployments are announced "+
		"separately instead of under one registry cause")
	inNamespace(t, "rootcause.shared-registry", func(s *Scenario) {
		const registry = "registry.kwatch-e2e.invalid"
		var blamed []string
		for index := 0; index < 4; index++ {
			name := fmt.Sprintf("service-%d", index)
			blamed = append(blamed, "deployment/"+s.Namespace+"/"+name)
			s.CreateDeployment(name, "sleep",
				withRemoteImage(registry+"/team/"+name+":1"))
		}
		s.WaitForPodReason(8*time.Minute, len(blamed),
			"ErrImagePull", "ImagePullBackOff")

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
	inNamespaceAlone(t, "rootcause.small-storm", func(s *Scenario) {
		s.CreateDeployment("storm", "crash", withReplicas(stormReplicas))
		s.WaitForPodReason(8*time.Minute, stormReplicas, "CrashLoopBackOff")

		s.ExpectRoot(harness.RootExpectation{
			Root:             "deployment/" + s.Namespace + "/storm",
			Tier:             "notify",
			MaxMessages:      3,
			MaxTotalMessages: 3,
			MustNotBlame:     s.ScheduledNodes(),
		})
	})
}
