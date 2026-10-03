//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioNodeRecovery(t *testing.T) {
	inNamespace(t, "node.recovery", func(s *Scenario) {
		nodeRequireKind(s)
		node := nodeToStop(s)
		nodeCreateSleepingPods(s, node, 3)
		s.Must(s.Env.Receiver.Clear(s.Ctx))

		nodeStopUntilNotReady(s, node)
		s.ExpectRoot(harness.RootExpectation{
			Root:         "node//" + node,
			Tier:         "page",
			MaxMessages:  2,
			MustNotBlame: []string{"pod/" + s.Namespace + "/node-impact-*"},
		})

		nodeStartUntilReady(s, node)
		s.ExpectKwatchHealthy()
	})
}
