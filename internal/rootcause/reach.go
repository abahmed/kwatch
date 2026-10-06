package rootcause

import "github.com/abahmed/kwatch/internal/inventory"

// maxReach bounds how many entities one Reach returns, so a query from
// a hub such as a namespace stays proportional to what the caller needs.
const maxReach = 256

// Reached is one entity found by Reach.
type Reached struct {
	ID inventory.EntityID
	// Via is the relation that led to it from the entity before it.
	Via inventory.RelationType
	// Hops is how many relations away from the start it is, at least 1.
	Hops int
	// Path runs from the start to ID, both included.
	Path []inventory.EntityID
}

// Reach lists what lies within hops relations of start, nearest first,
// each entity once by its shortest path. Outgoing follows what start
// depends on (upstream, where failure comes from); Incoming follows what
// depends on it (downstream, what a failure reaches). Only the listed
// relation types are followed, all of them when none is given. At most
// maxReach entities are returned, in a stable order.
func Reach(
	model inventory.Reader, start inventory.EntityID, dir inventory.Direction,
	hops int, types ...inventory.RelationType,
) []Reached {
	seen := map[inventory.EntityID]bool{start: true}
	frontier := []Reached{{ID: start, Path: []inventory.EntityID{start}}}
	var out []Reached
	for depth := 1; depth <= hops && len(frontier) > 0; depth++ {
		var next []Reached
		for _, from := range frontier {
			next = append(next, expand(model, from, dir, depth, types,
				seen)...)
		}
		out = append(out, next...)
		if len(out) >= maxReach {
			return out[:maxReach]
		}
		frontier = next
	}
	return out
}

// expand returns the unseen entities one relation away from from.
func expand(
	model inventory.Reader, from Reached, dir inventory.Direction,
	depth int, types []inventory.RelationType, seen map[inventory.EntityID]bool,
) []Reached {
	var out []Reached
	for _, r := range model.Relations(from.ID, dir) {
		to := r.To
		if dir == inventory.Incoming {
			to = r.From
		}
		if seen[to] || !allowedType(types, r.Type) {
			continue
		}
		seen[to] = true
		path := append(append([]inventory.EntityID(nil), from.Path...), to)
		out = append(out, Reached{ID: to, Via: r.Type, Hops: depth,
			Path: path})
	}
	return out
}

// allowedType reports a relation among types; no types allows all.
func allowedType(types []inventory.RelationType, t inventory.RelationType,
) bool {
	if len(types) == 0 {
		return true
	}
	for _, want := range types {
		if want == t {
			return true
		}
	}
	return false
}
