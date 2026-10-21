package incident

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// ambiguousChecks are the proof codes whose checks are about more than
// one workload: "all 6 replicas fail, on 3 nodes" for one workload and
// "all 5 replicas fail, on 3 nodes" for another read as the same check
// twice unless each says whose replicas it counts.
func (a *areaPlacer) ambiguousChecks(
	covers []inventory.EntityID,
) map[rootcause.ProofCode]bool {
	subjects := map[rootcause.ProofCode]map[inventory.EntityID]bool{}
	for _, failure := range covers {
		for _, cmp := range a.area.Trace.Compared[failure] {
			if cmp.Subject.IsZero() {
				continue
			}
			if subjects[cmp.Code] == nil {
				subjects[cmp.Code] = map[inventory.EntityID]bool{}
			}
			subjects[cmp.Code][cmp.Subject] = true
		}
	}
	out := map[rootcause.ProofCode]bool{}
	for code, set := range subjects {
		out[code] = len(set) > 1
	}
	return out
}

// checkText is the text of a check; it names the workload when other
// checks of its kind are about other workloads ("... on 3 nodes (api)").
func checkText(
	text string, cmp explain.Comparison, ambiguous map[rootcause.ProofCode]bool,
) string {
	if ambiguous[cmp.Code] && !cmp.Subject.IsZero() {
		return text + " (" + cmp.Subject.Name + ")"
	}
	return text
}
