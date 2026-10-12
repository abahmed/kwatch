package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// Each degraded state of a node explains the pods that fail on it, with
// the row and not another one.
func TestNodeDegradedModesExplainPodsOnTheNode(t *testing.T) {
	cases := map[detection.Mode]detection.Mode{
		detection.ModePLEGSlow:      detection.ModeNotReady,
		detection.ModeRuntimeErrors: detection.ModeCreateError,
		detection.ModePressureStall: detection.ModeProbe,
		detection.ModeEvicting:      detection.ModeEvicted,
		detection.NodeConditionModeOf("KernelDeadlock"): detection.
			ModeCrashLoop,
	}
	for nodeMode, effectMode := range cases {
		t.Run(string(nodeMode), func(t *testing.T) {
			f := newFixture(t)
			effect := nodeCase(nodeMode, effectMode)(f)
			c := requireCause(t, f.explain(), effect, "node//n1")
			if c.Row != "node-degraded" {
				t.Fatalf("row = %s, want node-degraded", c.Row)
			}
		})
	}
}

// A degraded node that hosts no failing pod is not blamed for a pod
// elsewhere, and a failure that is not one the node can cause is left
// to its own explanation.
func TestNodeDegradedNeedsAFailureOnTheNode(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	f.fail(nodes[0], detection.ModePLEGSlow, degradedH, 1, "")
	pods := f.workload("shop", "api", 2, nodes[1])
	f.fail(pods[0], detection.ModeNotReady, failingH, 2, "")

	if c, ok := f.explain().CauseOf(pods[0]); ok &&
		c.Root == nodes[0] {
		t.Fatalf("blamed %s for a pod on another node", c.Root)
	}
}
