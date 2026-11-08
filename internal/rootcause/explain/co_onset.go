package explain

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// onsetSpread is the workloads among effects whose start is known and
// how far apart the first and the last of those starts are. A workload
// starts when its earliest failing effect does.
func (v *view) onsetSpread(
	effects []inventory.EntityID,
) (workloads int, spread time.Duration) {
	starts := map[inventory.EntityID]time.Time{}
	for _, effect := range effects {
		at := v.earliestSince([]inventory.EntityID{effect})
		if !at.ok {
			continue
		}
		owner := v.workloadOf(effect)
		if first, seen := starts[owner]; !seen || at.t.Before(first) {
			starts[owner] = at.t
		}
	}
	var first, last time.Time
	for _, at := range starts {
		if first.IsZero() || at.Before(first) {
			first = at
		}
		if at.After(last) {
			last = at
		}
	}
	return len(starts), last.Sub(first)
}

// coOnset reports workloads that began failing within CoOnsetWindow of
// each other, at least CoOnsetMinWorkloads of them.
func (v *view) coOnset(effects []inventory.EntityID) (int, time.Duration, bool) {
	workloads, spread := v.onsetSpread(effects)
	return workloads, spread, workloads >= CoOnsetMinWorkloads &&
		spread <= CoOnsetWindow
}

// scoreCoOnset rewards a candidate that several workloads began failing
// under within seconds of each other. Failures that begin together
// share a cause more often than failures minutes apart. The reward is
// small and only corroborates: the candidate already needs evidence of
// its own, so a coincidence of clocks never makes a cause.
func scoreCoOnset(v *view, c *candidate) outcome {
	effects := c.others()
	workloads, spread, ok := v.coOnset(effects)
	if !ok {
		return outcome{}
	}
	return outcome{weight: CoOnsetWeight, code: rootcause.ProofCoOnset,
		count: workloads, total: int(spread.Seconds()),
		text: fmt.Sprintf("%d workloads began failing within %d seconds "+
			"of each other", workloads, int(spread.Seconds()))}
}
