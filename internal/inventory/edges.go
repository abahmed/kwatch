package inventory

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
	keys := make([]string, len(out))
	for i, relation := range out {
		keys[i] = relationKey(relation)
	}
	sortByKeys(out, keys)
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

// sortIDs orders ids by their String key. Each key is built once rather
// than twice per comparison, which matters for thousands of pods.
func sortIDs(ids []EntityID) {
	if len(ids) < 2 {
		return
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = id.String()
	}
	sortByKeys(ids, keys)
}

// sortByKeys sorts items by the matching keys, moving both together.
func sortByKeys[T any](items []T, keys []string) {
	sort.Sort(keyedSlice[T]{items: items, keys: keys})
}

type keyedSlice[T any] struct {
	items []T
	keys  []string
}

func (s keyedSlice[T]) Len() int           { return len(s.items) }
func (s keyedSlice[T]) Less(i, j int) bool { return s.keys[i] < s.keys[j] }
func (s keyedSlice[T]) Swap(i, j int) {
	s.items[i], s.items[j] = s.items[j], s.items[i]
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
}
