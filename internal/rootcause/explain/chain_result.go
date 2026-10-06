package explain

import (
	"math"
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
)

// result builds the hops of the deepest live chain of a cause. It is
// empty when the chain reached nothing.
func (cx *chainCtx) result(
	s scored, list []reached, beyond []Hop,
) chainResult {
	best := -1
	for i, r := range list {
		if r.depth == 0 || r.dead {
			continue
		}
		if best < 0 || r.depth > list[best].depth ||
			(r.depth == list[best].depth &&
				cx.earlier(list[best].anchor, r.anchor)) {
			best = i
		}
	}
	if best < 0 {
		return chainResult{}
	}
	var hops []Hop
	for i := best; i >= 0; i = list[i].parent {
		conf := s.confidence * math.Pow(ChainDecay, float64(list[i].depth))
		hops = append([]Hop{cx.hop(list[i].anchor, list[i].edge.link,
			conf)}, hops...)
	}
	if root := s.c.id; hops[0].Entity != root {
		hops = append([]Hop{cx.hop(root, "", s.confidence)}, hops...)
	}
	hops[0].Link = ""
	sort.SliceStable(beyond, func(i, j int) bool {
		return beyond[i].Entity.String() < beyond[j].Entity.String()
	})
	return chainResult{hops: hops, beyond: beyond}
}

// earlier orders two anchors by when they began, then by key. Of two
// ends of equal depth the later one is shown: it is further down.
func (cx *chainCtx) earlier(a, b inventory.EntityID) bool {
	ta, tb := cx.graph.began[a], cx.graph.began[b]
	if !ta.Equal(tb) {
		return ta.Before(tb)
	}
	return a.String() < b.String()
}

// hop builds one hop for an entity.
func (cx *chainCtx) hop(
	id inventory.EntityID, link LinkType, confidence float64,
) Hop {
	began := cx.graph.began[cx.v.anchorOf(id)]
	if began.IsZero() {
		began = cx.v.began(id)
	}
	return Hop{Entity: id, Began: began, Link: link, Confidence: confidence}
}
