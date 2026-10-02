package explain

import "github.com/abahmed/kwatch/internal/inventory"

// areasOf groups failures into connected areas: two failures share an
// area when one viable candidate covers both. A failure no candidate
// covers is an area of its own. Areas come out in the order of their
// first failure, and each lists its failures sorted.
func areasOf(
	failures []inventory.EntityID, viable []scored,
) [][]inventory.EntityID {
	parent := map[inventory.EntityID]inventory.EntityID{}
	for _, f := range failures {
		parent[f] = f
	}
	var find func(inventory.EntityID) inventory.EntityID
	find = func(id inventory.EntityID) inventory.EntityID {
		for parent[id] != id {
			parent[id] = parent[parent[id]]
			id = parent[id]
		}
		return id
	}
	for _, s := range viable {
		var first inventory.EntityID
		for _, effect := range sortedKeys(s.c.covers) {
			if _, ok := parent[effect]; !ok {
				continue
			}
			if first.IsZero() {
				first = effect
				continue
			}
			parent[find(effect)] = find(first)
		}
	}
	index := map[inventory.EntityID]int{}
	var out [][]inventory.EntityID
	for _, f := range failures {
		root := find(f)
		i, ok := index[root]
		if !ok {
			i = len(out)
			index[root] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], f)
	}
	return out
}
