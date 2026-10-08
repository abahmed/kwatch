package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultLeftoverAge is how long a deletion may be stuck before an
// object nothing uses is a leftover, not news. A fresh stuck deletion
// is worth a message: the person who deleted it is probably waiting.
const DefaultLeftoverAge = 24 * time.Hour

// liveDependents are the relations by which another object depends on
// the one being deleted: it uses it, mounts it, serves it or is owned
// by it.
var liveDependents = []inventory.RelationType{
	inventory.References, inventory.Mounts, inventory.OwnedBy,
	inventory.Serves, inventory.RoutesTo, inventory.Scales,
}

// leftover reports a deletion that has been stuck longer than
// DefaultLeftoverAge and holds nothing live: no object that is not
// itself being deleted depends on it. Such a deletion is housekeeping;
// one that blocks a live object keeps its severity.
func leftover(
	ctx detection.Context, e inventory.Entity, since time.Time,
) bool {
	if since.IsZero() || ctx.Now.Sub(since) < DefaultLeftoverAge ||
		ctx.Model == nil {
		return false
	}
	for _, relation := range liveDependents {
		for _, from := range ctx.Model.Related(
			e.ID, relation, inventory.Incoming) {
			if live, found := ctx.Model.Entity(from); found &&
				!flag(live, kube.AttrDeleting) {
				return false
			}
		}
	}
	return true
}
