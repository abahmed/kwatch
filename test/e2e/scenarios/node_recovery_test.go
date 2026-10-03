//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioNodeRecovery(t *testing.T) {
	inNamespace(t, "node.recovery", func(s *Scenario) {
		s.RequireKind()
		node := s.FreeWorkerNode()
		s.CreateSleepingPods(node, 3)
		s.ClearReceiver()

		s.StopNode(node)
		s.ExpectRoot(harness.RootExpectation{
			Root:         "node//" + node,
			Tier:         "page",
			MaxMessages:  2,
			MustNotBlame: []string{"pod/" + s.Namespace + "/node-impact-*"},
		})

		s.StartNode(node)
		s.ExpectKwatchHealthy()
	})
}
