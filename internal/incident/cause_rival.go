package incident

import (
	"math"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// maxCheckedKept bounds the counterfactual checks a cause remembers.
const maxCheckedKept = 3

// annotate adds to a cause record what a message needs to word it
// honestly: the rival cause that scored almost as well, and what the
// counterfactual checks compared the failures with.
func (a *areaPlacer) annotate(record *rootcause.CauseRecord, c explain.Cause) {
	record.Checked = a.checkedTexts(c.Covers, record.Proof)
	for _, alt := range a.area.Alternatives {
		if alt.Root == c.Root || !shareFailure(alt.Covers, c.Covers) ||
			slices.Contains(c.Covers, alt.Root) ||
			slices.Contains(c.Chain, alt.Root) ||
			slices.Contains(alt.Chain, c.Root) {
			// One cause upstream of the other is the same story told
			// at two depths, not a second explanation.
			continue
		}
		if math.Abs(c.Confidence-alt.Confidence) <=
			rootcause.RivalMargin+1e-9 {
			rival := a.s.Record(alt)
			record.Rival = &rival
		}
		return
	}
}

// shareFailure reports two causes that explain some failure in common.
func shareFailure(a, b []inventory.EntityID) bool {
	for _, id := range a {
		if slices.Contains(b, id) {
			return true
		}
	}
	return false
}

// checkedTexts is what the checks compared the failures with, without
// repeats. Checks that gave the chosen cause its own proof come first;
// within each group the order of the failures and of the checks holds.
func (a *areaPlacer) checkedTexts(
	covers []inventory.EntityID, proof []rootcause.Proof,
) []string {
	var own, other []string
	for _, failure := range covers {
		for _, cmp := range a.area.Trace.Compared[failure] {
			text := strings.TrimSpace(cmp.Text)
			if text == "" || slices.Contains(own, text) ||
				slices.Contains(other, text) {
				continue
			}
			if slices.ContainsFunc(proof, func(p rootcause.Proof) bool {
				return p.Code == cmp.Code
			}) {
				own = append(own, text)
			} else {
				other = append(other, text)
			}
		}
	}
	return slices.Concat(own, other)[:min(maxCheckedKept,
		len(own)+len(other))]
}
