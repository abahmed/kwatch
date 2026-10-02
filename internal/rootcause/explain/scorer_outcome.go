package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreOutcome reads how the candidate's latest change turned out. A
// revert that brought its dependents back confirms the change as the
// cause; dependents that failed inside its effect window support it.
// A healthy outcome says nothing: the inventory's blast radius follows
// stored relations only, so dependents reached through read-time links
// (a policy selecting pods) look healthy when they are not. Without
// outcome data it says nothing.
func scoreOutcome(v *view, c *candidate) outcome {
	if v.s.Outcomes == nil {
		return outcome{}
	}
	changes := v.changesOf(c.id)
	if len(changes) == 0 {
		return outcome{}
	}
	latest := changes[len(changes)-1]
	result, ok := v.s.Outcomes.Outcome(latest, v.s.Now)
	if !ok {
		return outcome{}
	}
	switch result {
	case inventory.OutcomeReverted:
		return outcome{weight: OutcomeRevertedWeight,
			code: rootcause.ProofRevertRecovered,
			text: "reverting its change brought its dependents back"}
	case inventory.OutcomeDegraded:
		return outcome{weight: OutcomeDegradedWeight,
			code: rootcause.ProofFailedAfterChange,
			text: "its dependents failed soon after it changed"}
	}
	return outcome{}
}
