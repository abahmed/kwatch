package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// The node rows read the detector's OOM verdict: a kill the node caused
// is the node's, a kill at the container's own limit or without evidence
// either way is not, whatever pressure the node reports.
func TestExplainOOMVerdictDecidesWhetherTheNodeIsBlamed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   detection.Mode
		blamed bool
	}{
		{"killed by the node", detection.ModeOOMKilledNode, true},
		{"killed at its own limit", detection.ModeOOMKilledLimit, false},
		{"no verdict", detection.ModeOOMKilled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			effect := nodeCase("MemoryPressure", tc.mode)(f)

			c, ok := f.explain().CauseOf(effect)

			got := ok && c.Root.String() == "node//n1"
			if got != tc.blamed {
				t.Fatalf("node blamed = %v, want %v (cause %v)", got,
					tc.blamed, c.Root)
			}
		})
	}
}
