package pipeline

import "github.com/abahmed/kwatch/internal/inventory"

// syncWaits remembers the entities evaluated while a kind they needed was
// not synced. A detector must not conclude that an object is missing
// before its kind was listed, so such an entity may have been judged
// healthy only for lack of data. An object that does not exist raises no
// event when its kind finishes syncing, so without this list nothing
// would ever evaluate the entity again: a webhook calling a Service that
// does not exist stays unreported until someone edits it.
type syncWaits struct {
	byKind map[inventory.Kind]map[inventory.EntityID]struct{}
}

// record replaces what id waits for with the kinds of its last
// evaluation.
func (w *syncWaits) record(id inventory.EntityID, kinds []inventory.Kind) {
	for kind, ids := range w.byKind {
		delete(ids, id)
		if len(ids) == 0 {
			delete(w.byKind, kind)
		}
	}
	for _, kind := range kinds {
		if w.byKind == nil {
			w.byKind = map[inventory.Kind]map[inventory.EntityID]struct{}{}
		}
		if w.byKind[kind] == nil {
			w.byKind[kind] = map[inventory.EntityID]struct{}{}
		}
		w.byKind[kind][id] = struct{}{}
	}
}

// ready removes and returns the entities waiting for kinds that are
// synced now.
func (w *syncWaits) ready(
	synced func(inventory.Kind) bool,
) []inventory.EntityID {
	var out []inventory.EntityID
	for kind, ids := range w.byKind {
		if synced != nil && !synced(kind) {
			continue
		}
		for id := range ids {
			out = append(out, id)
		}
		delete(w.byKind, kind)
	}
	return out
}
