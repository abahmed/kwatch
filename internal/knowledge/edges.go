package knowledge

import "sort"

// edgeIndex stores directed edges in both directions with reference
// counts, so several sources may declare the same edge and it disappears
// only when the last source withdraws it.
type edgeIndex struct {
	out map[EntityID]map[RelationType]map[EntityID]int
	in  map[EntityID]map[RelationType]map[EntityID]int
	n   int
}

func newEdgeIndex() edgeIndex {
	return edgeIndex{
		out: make(map[EntityID]map[RelationType]map[EntityID]int),
		in:  make(map[EntityID]map[RelationType]map[EntityID]int),
	}
}

func (x *edgeIndex) add(from EntityID, rel RelationType, to EntityID) {
	if increment(x.out, from, rel, to) == 1 {
		x.n++
	}
	increment(x.in, to, rel, from)
}

func (x *edgeIndex) remove(from EntityID, rel RelationType, to EntityID) {
	if decrement(x.out, from, rel, to) == 0 {
		x.n--
	}
	decrement(x.in, to, rel, from)
}

func (x *edgeIndex) neighbors(
	id EntityID, rel RelationType, dir Direction,
) []EntityID {
	side := x.out
	if dir == Incoming {
		side = x.in
	}
	targets := side[id][rel]
	out := make([]EntityID, 0, len(targets))
	for target := range targets {
		out = append(out, target)
	}
	sortIDs(out)
	return out
}

func (x *edgeIndex) relations(id EntityID, dir Direction) []Relation {
	side := x.out
	if dir == Incoming {
		side = x.in
	}
	var out []Relation
	for rel, targets := range side[id] {
		for target := range targets {
			if dir == Outgoing {
				out = append(out, Relation{From: id, Type: rel, To: target})
			} else {
				out = append(out, Relation{From: target, Type: rel, To: id})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return relationKey(out[i]) < relationKey(out[j])
	})
	return out
}

func increment(
	side map[EntityID]map[RelationType]map[EntityID]int,
	a EntityID, rel RelationType, b EntityID,
) int {
	byType := side[a]
	if byType == nil {
		byType = make(map[RelationType]map[EntityID]int)
		side[a] = byType
	}
	targets := byType[rel]
	if targets == nil {
		targets = make(map[EntityID]int)
		byType[rel] = targets
	}
	targets[b]++
	return targets[b]
}

func decrement(
	side map[EntityID]map[RelationType]map[EntityID]int,
	a EntityID, rel RelationType, b EntityID,
) int {
	targets := side[a][rel]
	if targets[b] == 0 {
		return -1
	}
	targets[b]--
	count := targets[b]
	if count > 0 {
		return count
	}
	delete(targets, b)
	if len(targets) == 0 {
		delete(side[a], rel)
	}
	if len(side[a]) == 0 {
		delete(side, a)
	}
	return 0
}

func relationKey(r Relation) string {
	return r.From.String() + "|" + string(r.Type) + "|" + r.To.String()
}

func sortIDs(ids []EntityID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}
