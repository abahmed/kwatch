package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ChangesBetween returns the changes worth naming that people made from
// from to to on the root, the objects it uses, what routes to or scales
// it and the nodes its pods run on: the newest per object, nearest to
// the root first and newest first within one distance, at most limit.
// The dashboard lists them as what changed around an incident. Without
// a model, or for a root outside a namespace, it returns nothing.
func ChangesBetween(
	model inventory.HistoryReader, root inventory.EntityID,
	from, to time.Time, limit int,
) []inventory.Change {
	if limit <= 0 {
		return nil
	}
	return rankedUpTo(model, root, from, to, nearNamespace, limit)
}
