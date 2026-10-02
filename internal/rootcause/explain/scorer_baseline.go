package explain

import (
	"fmt"
	"math"

	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreBaseline moves confidence by how unusual the candidate looks
// against its learned baseline. Without a baseline it says nothing.
func scoreBaseline(v *view, c *candidate) outcome {
	if v.s.Baseline == nil {
		return outcome{}
	}
	deviation, ok := v.s.Baseline.Deviation(c.id)
	if !ok {
		return outcome{}
	}
	deviation = min(max(deviation, 0), 1)
	percent := math.RoundToEven(deviation * 100)
	return outcome{weight: BaselineWeight * (2*deviation - 1),
		code: rootcause.ProofBaseline, count: int(percent),
		text: fmt.Sprintf("it deviates %.0f%% from its baseline",
			deviation*100)}
}
