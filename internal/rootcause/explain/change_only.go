package explain

import (
	"slices"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// discriminating are the proofs that tie a failure to a changed
// dependency: its name or key in the errors, an error of the kind such a
// dependency produces, a revert that cured it, a split between pods that
// read the old and the new content.
var discriminating = map[rootcause.ProofCode]bool{
	rootcause.ProofErrorsName:      true,
	rootcause.ProofErrorFits:       true,
	rootcause.ProofRevertRecovered: true,
	rootcause.ProofNewConfigFails:  true,
	rootcause.ProofOldConfigFails:  true,
}

// changeOnlyDependency reports a candidate that is not a workload, is
// matched only as a change, has no discriminating proof, and whose
// failures quote a defect of their own program. An edit of what a
// workload reads may still have triggered such a defect, but nothing in
// the words says so.
func (v *view) changeOnlyDependency(c *candidate, got []Contribution) bool {
	if c.isSelf() || slices.Contains(kube.WorkloadKinds, c.id.Kind) {
		return false
	}
	changed := false
	for _, how := range c.covers {
		if how.derived || how.match.row.Name == selfRow.Name {
			continue
		}
		if !how.match.causeMode.Within(ModeChanged) {
			return false
		}
		changed = true
	}
	for _, g := range got {
		if g.Weight > 0 && discriminating[g.Code] {
			return false
		}
	}
	return changed && v.errorsAreProgramDefects(c)
}

// errorsAreProgramDefects reports that at least half of the failures the
// candidate explains quote a defect of their own program.
func (v *view) errorsAreProgramDefects(c *candidate) bool {
	effects := c.others()
	named := 0
	for _, effect := range effects {
		if quotesProgramDefect(v.errorText(effect)) {
			named++
		}
	}
	return len(effects) > 0 && named*2 >= len(effects)
}
