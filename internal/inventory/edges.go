package inventory

import "sort"

// edgeIndex stores directed edges in both directions with reference
// counts, so several sources may declare the same edge and it disappears
// only when the last source withdraws it.
type edgeIndex struct {
	out edgeSide
	in  edgeSide
	n   int
}

// edgeSide maps an entity to its neighbours, grouped by relation type.
// An entity has few relation types, so they sit in a slice.
type edgeSide map[EntityID][]typedEdges

type typedEdges struct {
	rel     RelationType
	targets edgeSet
}

func newEdgeIndex() edgeIndex {
	return edgeIndex{out: make(edgeSide), in: make(edgeSide)}
}

func (x *edgeIndex) add(from EntityID, rel RelationType, to EntityID) {
	if x.out.increment(from, rel, to) == 1 {
		x.n++
	}
	x.in.increment(to, rel, from)
}

func (x *edgeIndex) remove(from EntityID, rel RelationType, to EntityID) {
	if x.out.decrement(from, rel, to) == 0 {
		x.n--
	}
	x.in.decrement(to, rel, from)
}

func (x *edgeIndex) side(dir Direction) edgeSide {
	if dir == Incoming {
		return x.in
	}
	return x.out
}

func (x *edgeIndex) neighbors(
	id EntityID, rel RelationType, dir Direction,
) []EntityID {
	var out []EntityID
	for _, typed := range x.side(dir)[id] {
		if typed.rel == rel {
			out = typed.targets.ids()
		}
	}
	if out == nil {
		out = []EntityID{}
	}
	sortIDs(out)
	return out
}

func (x *edgeIndex) relations(id EntityID, dir Direction) []Relation {
	var out []Relation
	for _, typed := range x.side(dir)[id] {
		for _, target := range typed.targets.ids() {
			if dir == Outgoing {
				out = append(out,
					Relation{From: id, Type: typed.rel, To: target})
			} else {
				out = append(out,
					Relation{From: target, Type: typed.rel, To: id})
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

func (s edgeSide) increment(a EntityID, rel RelationType, b EntityID) int {
	for i := range s[a] {
		if s[a][i].rel == rel {
			return s[a][i].targets.inc(b)
		}
	}
	typed := typedEdges{rel: rel}
	count := typed.targets.inc(b)
	s[a] = append(s[a], typed)
	return count
}

// decrement withdraws one source of the edge and returns the sources
// left, or -1 when the edge was not there. Emptied groups disappear.
func (s edgeSide) decrement(a EntityID, rel RelationType, b EntityID) int {
	groups := s[a]
	for i := range groups {
		if groups[i].rel != rel {
			continue
		}
		left := groups[i].targets.dec(b)
		if left == 0 && groups[i].targets.size() == 0 {
			groups = append(groups[:i], groups[i+1:]...)
			if len(groups) == 0 {
				delete(s, a)
			} else {
				s[a] = groups
			}
		}
		return left
	}
	return -1
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
