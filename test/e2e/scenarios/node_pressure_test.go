//go:build e2e

package scenarios

import (
	"testing"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

func TestScenarioNodePressureSignals(t *testing.T) {
	onCluster(t, "node.pressure-signals", func(s *Scenario) {
		nodeRequireKind(s)
		node := nodeToStop(s)
		nodeStopUntilNotReady(s, node)
		nodePressureConditions(s, node)
		s.ExpectRoot(harness.RootExpectation{
			Root:        "node//" + node,
			MaxMessages: 4,
		})

		nodeStartUntilReady(s, node)
		s.ExpectKwatchHealthy()
	})
}
