package explain

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// sortIDs sorts ids by their canonical key.
func sortIDs(ids []inventory.EntityID) {
	slices.SortFunc(ids, inventory.CompareIDs)
}

// failures lists the entities with a Failing or Degraded finding.
func failures(findings map[inventory.EntityID][]detection.Finding,
) []inventory.EntityID {
	var out []inventory.EntityID
	for id, found := range findings {
		for _, f := range found {
			if unhealthy(f) {
				out = append(out, id)
				break
			}
		}
	}
	sortIDs(out)
	return out
}

// sharedFactorOnlyState reports a cause that is only suspected as what
// the failures share, not one with a finding of its own: a zone that
// holds every failing replica reaches them through healthy nodes.
func sharedFactorOnlyState(s state) bool {
	for _, m := range s.modes {
		if m.mode != ModeSharedFactor {
			return false
		}
	}
	return len(s.modes) > 0
}
