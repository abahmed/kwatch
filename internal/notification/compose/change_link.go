package compose

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// linkFact is the clause of a change that records the end of a link:
// "service payments stopped routing to api at 10:02 by alice". ok is
// false for any other change.
func linkFact(change inventory.Change, who, at string) (string, bool) {
	words, target, ok := incident.EndedLink(change)
	if !ok {
		return "", false
	}
	return shortName(change.Entity) + " " + words + " " +
		shortName(target) + at + authoredBy(who), true
}
