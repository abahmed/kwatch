package inventory

// edgeSetSmall is how many targets an edgeSet keeps in a slice before it
// switches to a map. Most entities have one or two neighbours of a type,
// and a map costs hundreds of bytes even when it holds one entry.
const edgeSetSmall = 8

type edgeCount struct {
	id EntityID
	n  int
}

// edgeSet counts how many sources declared each edge to a neighbour. It
// is a slice while small and a map once a node has many pods.
type edgeSet struct {
	few  []edgeCount
	many map[EntityID]int
}

func (s *edgeSet) size() int {
	if s.many != nil {
		return len(s.many)
	}
	return len(s.few)
}

// inc counts one more source for id and returns the new count.
func (s *edgeSet) inc(id EntityID) int {
	if s.many != nil {
		s.many[id]++
		return s.many[id]
	}
	for i := range s.few {
		if s.few[i].id == id {
			s.few[i].n++
			return s.few[i].n
		}
	}
	if len(s.few) < edgeSetSmall {
		s.few = append(s.few, edgeCount{id: id, n: 1})
		return 1
	}
	s.many = make(map[EntityID]int, 2*edgeSetSmall)
	for _, c := range s.few {
		s.many[c.id] = c.n
	}
	s.few = nil
	s.many[id] = 1
	return 1
}

// dec counts one source fewer for id and returns what is left, or -1
// when id was not there.
func (s *edgeSet) dec(id EntityID) int {
	if s.many != nil {
		n := s.many[id]
		if n == 0 {
			return -1
		}
		if n == 1 {
			delete(s.many, id)
			return 0
		}
		s.many[id] = n - 1
		return n - 1
	}
	for i := range s.few {
		if s.few[i].id != id {
			continue
		}
		if s.few[i].n > 1 {
			s.few[i].n--
			return s.few[i].n
		}
		last := len(s.few) - 1
		s.few[i] = s.few[last]
		s.few[last] = edgeCount{}
		s.few = s.few[:last]
		return 0
	}
	return -1
}

// ids lists the neighbours in no particular order.
func (s *edgeSet) ids() []EntityID {
	out := make([]EntityID, 0, s.size())
	if s.many != nil {
		for id := range s.many {
			out = append(out, id)
		}
		return out
	}
	for _, c := range s.few {
		out = append(out, c.id)
	}
	return out
}
