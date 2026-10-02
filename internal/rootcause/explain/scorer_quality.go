package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreQuality lowers confidence when the data is doubtful: a finding
// of Unknown health on the candidate or its effects, or a candidate
// called missing in a kind kwatch does not fully watch. It never
// raises confidence.
func scoreQuality(v *view, c *candidate) outcome {
	var doubts []string
	if v.anyUnknown(append([]inventory.EntityID{c.id}, c.direct()...)) {
		doubts = append(doubts, "some health is unknown")
	}
	if v.calledMissing(c) && !v.s.synced(c.id.Kind) {
		doubts = append(doubts, "its kind is not fully watched, so "+
			"it may exist")
	}
	if len(doubts) == 0 {
		return outcome{}
	}
	return outcome{weight: -DataQualityPenalty * float64(len(doubts)),
		code: rootcause.ProofDoubtfulData, text: strings.Join(doubts, "; ")}
}

// anyUnknown reports whether one of ids has an Unknown finding.
func (v *view) anyUnknown(ids []inventory.EntityID) bool {
	for _, id := range ids {
		if v.hasUnknown(id) {
			return true
		}
	}
	return false
}

// calledMissing reports whether a row matched the candidate as absent.
func (v *view) calledMissing(c *candidate) bool {
	for _, how := range c.covers {
		if how.match.causeMode == ModeMissing {
			return true
		}
	}
	return false
}
