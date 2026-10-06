package explain

import (
	"math"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// A failure often travels. A database runs out of memory, the API that
// calls it stops being ready, the Service in front of the API loses its
// endpoints. Each step has a failure of its own, and the walk upstream
// from the last one stops at the first step whose error text does not
// name the one before it. Chains close that gap: once a cause is
// chosen, it is extended down the failure chain, one step at a time, to
// the failures that follow from it (chain_edges.go says what a step is).

// Chain limits. Every step needs evidence and time order; these only
// say how far a chain may run and how far it is trusted.
const (
	// ChainMaxHops is the most steps a chain reaches past what a cause
	// explains directly. A failure one step further is told as lying
	// beyond the chain, never claimed.
	ChainMaxHops = 3
	// ChainDecay multiplies the cause's confidence at each step. A step
	// is one more place for two unrelated failures to coincide, so each
	// is trusted a tenth less than the one before. With the floor at
	// 0.45, a cause at 0.70 reaches its third step (0.51) and one at
	// 0.60 stops after its second (0.49).
	ChainDecay = 0.9
)

// Hop is one step of a failure chain: the entity that failed, when it
// began to, and how the step before it led to it.
type Hop struct {
	// Entity is a workload, a Service or a route, never a pod or a
	// container: those are named by their workload.
	Entity inventory.EntityID
	// Began is when the entity's earliest failure started.
	Began time.Time
	// Link is how the previous hop led here: calls (this entity calls
	// the previous one), backs (the previous one backs this Service) or
	// routes-to (this route sends traffic to the previous one). Empty
	// for the first hop.
	Link LinkType `json:",omitempty"`
	// Confidence is the cause's confidence after ChainDecay for every
	// step so far; the first hop has the cause's own.
	Confidence float64
}

// chainResult is the chain a cause was extended along.
type chainResult struct {
	// hops run from the cause to the deepest entity it reached.
	hops []Hop
	// beyond are the entities a step past ChainMaxHops would reach.
	beyond []Hop
}

// chainRow stands in for a row on the coverage a chain adds. The
// coverage is derived: it counts for the set cover, never as evidence.
var chainRow = Row{Name: "chain"}

// reached is one anchor a chain reached.
type reached struct {
	anchor inventory.EntityID
	edge   chainEdge
	depth  int
	parent int // index in the reached list, -1 for a start
	// dead marks an anchor whose failures a chain could not take; it
	// is neither shown nor left from.
	dead bool
}

// chainCtx is what extending the chains of one solve reads.
type chainCtx struct {
	v     *view
	graph *chainGraph
	inAll map[inventory.EntityID]bool
	// specificBy lists, per failure, the specific causes that explain
	// it directly: it is theirs, not a chain's.
	specificBy map[inventory.EntityID][]inventory.EntityID
	// ownCause marks the workloads with a cause of their own: a change
	// or a fault of their own such as a memory limit too low.
	ownCause map[inventory.EntityID]bool
	// claimed are the failures a chain has taken so far.
	claimed map[inventory.EntityID]bool
	// reached are the anchors some chain has reached so far.
	reached map[inventory.EntityID]bool
}

// extendChains extends every viable cause along the failure chains of
// graph, in the order of their score. A chain covers a failure only
// when no other specific cause explains it and its workload has no cause
// of its own. It returns the candidates that still explain something:
// a failure with a cause upstream is no longer its own workload's.
func (v *view) extendChains(
	viable []scored, graph *chainGraph, all []inventory.EntityID,
) []scored {
	if graph.empty() {
		return viable
	}
	cx := &chainCtx{v: v, graph: graph, inAll: map[inventory.EntityID]bool{},
		specificBy: map[inventory.EntityID][]inventory.EntityID{},
		ownCause:   map[inventory.EntityID]bool{},
		claimed:    map[inventory.EntityID]bool{},
		reached:    map[inventory.EntityID]bool{}}
	for _, id := range all {
		cx.inAll[id] = true
	}
	cx.index(viable)
	for _, s := range cx.upstreamFirst(viable) {
		if !cx.isDownstream(s) {
			cx.grow(s)
		}
	}
	return cx.withoutClaimed(viable)
}

// upstreamFirst orders the candidates by when they began, so a chain
// leaves from the first failure and not from one of its consequences.
// Candidates that began together keep their order of score.
func (cx *chainCtx) upstreamFirst(viable []scored) []scored {
	out := append([]scored(nil), viable...)
	began := func(s scored) time.Time {
		t, ok := cx.graph.began[cx.v.anchorOf(s.c.id)]
		if !ok {
			return time.Time{}
		}
		return t
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := began(out[i]), began(out[j])
		if a.IsZero() || b.IsZero() {
			return !a.IsZero() && b.IsZero()
		}
		return a.Before(b)
	})
	return out
}

// isDownstream reports a candidate that an earlier chain has reached:
// its failures are consequences of that chain, not a cause to extend.
func (cx *chainCtx) isDownstream(s scored) bool {
	return cx.reached[cx.v.anchorOf(s.c.id)]
}

// index reads which failures specific causes explain and which
// workloads have a cause of their own.
func (cx *chainCtx) index(viable []scored) {
	for _, s := range viable {
		if s.c.isSelf() || sharedFactorOnly(s.c) {
			continue
		}
		for _, effect := range s.c.direct() {
			cx.specificBy[effect] = append(cx.specificBy[effect], s.c.id)
		}
		for _, how := range s.c.covers {
			if how.link == LinkSelf {
				cx.ownCause[s.c.id] = true
			}
		}
	}
}

// hasOwnCause reports a workload that explains its own failure: a
// specific fault of its own, or a change inside the causal window.
func (cx *chainCtx) hasOwnCause(anchor inventory.EntityID) bool {
	return cx.ownCause[anchor] || len(cx.v.changesOf(anchor)) > 0
}

// grow walks the chain graph out of one cause, nearest first, and adds
// the derived coverage and the chain.
func (cx *chainCtx) grow(s scored) {
	var list []reached
	seen := map[inventory.EntityID]bool{}
	for _, anchor := range cx.starts(s) {
		seen[anchor] = true
		list = append(list, reached{anchor: anchor, parent: -1})
	}
	var beyond []Hop
	for i := 0; i < len(list); i++ {
		if list[i].dead {
			continue
		}
		for _, edge := range cx.graph.out[list[i].anchor] {
			if seen[edge.to] || cx.hasOwnCause(edge.to) {
				continue
			}
			depth := list[i].depth + 1
			switch {
			case depth > ChainMaxHops:
				beyond = append(beyond, cx.hop(edge.to, edge.link, 0))
				seen[edge.to] = true
			case cx.trusted(s, depth):
				seen[edge.to] = true
				list = append(list, cx.step(s, list, i, edge, seen))
			}
		}
	}
	s.c.chain = cx.result(s, list, beyond)
}

// trusted reports whether a cause is still above the floor after depth
// steps of ChainDecay.
func (cx *chainCtx) trusted(s scored, depth int) bool {
	return s.confidence*math.Pow(ChainDecay, float64(depth)) >=
		ConfidenceFloor
}

// step reaches edge.to from list[from], takes the failures there, and
// returns the anchor reached; it is dead when nothing could be taken.
func (cx *chainCtx) step(
	s scored, list []reached, from int, edge chainEdge,
	seen map[inventory.EntityID]bool,
) reached {
	next := reached{anchor: edge.to, edge: edge, depth: list[from].depth + 1,
		parent: from}
	list = append(list, next)
	next.dead = !cx.cover(s, list, len(list)-1, seen)
	cx.reached[edge.to] = cx.reached[edge.to] || !next.dead
	return next
}

// starts are the anchors the cause already explains, which a chain
// leaves from: those of its coverage and its own.
func (cx *chainCtx) starts(s scored) []inventory.EntityID {
	set := map[inventory.EntityID]bool{cx.v.anchorOf(s.c.id): true}
	for effect := range s.c.covers {
		set[cx.v.anchorOf(effect)] = true
	}
	return sortedKeys(set)
}

// cover gives the cause derived coverage of the failures of one reached
// anchor. It reports false when none could be taken.
func (cx *chainCtx) cover(
	s scored, list []reached, at int, seen map[inventory.EntityID]bool,
) bool {
	anchor := list[at].anchor
	path := cx.path(list, at)
	// An anchor with no finding yet has nothing to take, but the chain
	// still runs through it.
	taken := len(cx.graph.members[anchor]) == 0
	for _, failure := range cx.graph.members[anchor] {
		if !cx.inAll[failure] || cx.claimed[failure] ||
			cx.hasOtherFailure(failure) ||
			cx.takenElsewhere(failure, s.c.id, seen) {
			continue
		}
		s.c.covers[failure] = coverage{derived: true,
			match: rowMatch{row: chainRow}, chain: path}
		cx.claimed[failure] = true
		taken = true
	}
	return taken
}

// hasOtherFailure reports a pod or container that fails in a way a
// failed call does not explain, such as an OOM kill: it has a cause of
// its own, whatever its workload's other pods do.
func (cx *chainCtx) hasOtherFailure(failure inventory.EntityID) bool {
	unit, ok := cx.v.unitOf(failure)
	if !ok {
		return false
	}
	ids := append([]inventory.EntityID{unit}, cx.v.s.Model.Related(
		unit, inventory.PartOf, inventory.Incoming)...)
	for _, id := range ids {
		for _, f := range cx.v.s.Findings[id] {
			if unhealthy(f) && !anyModeMatches(chainModes, f.Mode) {
				return true
			}
		}
	}
	return false
}

// takenElsewhere reports a specific cause, other than id and the
// anchors the chain has reached, that explains the failure. A pod of
// the chain that explains its Service is a step of the chain, not a
// rival of it.
func (cx *chainCtx) takenElsewhere(
	failure, id inventory.EntityID, seen map[inventory.EntityID]bool,
) bool {
	for _, other := range cx.specificBy[failure] {
		if other != id && !seen[cx.v.anchorOf(other)] {
			return true
		}
	}
	return false
}

// path lists the entities from the cause's first anchor to reached[at].
func (cx *chainCtx) path(list []reached, at int) []inventory.EntityID {
	var out []inventory.EntityID
	for i := at; i >= 0; i = list[i].parent {
		out = append([]inventory.EntityID{list[i].anchor}, out...)
	}
	return out
}

// withoutClaimed takes the failures chains claimed out of the other
// self-only candidates and drops the candidates left with nothing.
func (cx *chainCtx) withoutClaimed(viable []scored) []scored {
	if len(cx.claimed) == 0 {
		return viable
	}
	out := viable[:0]
	for _, s := range viable {
		if s.c.isSelf() {
			for effect := range cx.claimed {
				if how, ok := s.c.covers[effect]; ok &&
					how.match.row.Name != chainRow.Name {
					delete(s.c.covers, effect)
				}
			}
			if len(s.c.covers) == 0 {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// chainModes are the failures a caller may show for a call to fail:
// what a failed call does to it, and not being ready. A pod with
// another failure, such as OOMKilled, has a cause of its own.
var chainModes = append(append([]detection.Mode(nil), callerFailures...),
	detection.ModeNotReady, detection.ModeProbe)

// noteChain records the chain of the first cause that has one in the
// area's trace, and the Services its steps read among the area's inputs.
func (a *Area) noteChain(graph *chainGraph) {
	for _, cause := range a.Causes {
		if len(cause.Hops) > 0 {
			a.Trace.Chain = cause.Hops
			break
		}
	}
	extra := graph.inputsOf(a.Failures)
	if len(extra) == 0 {
		return
	}
	merged := map[inventory.EntityID]bool{}
	for _, id := range append(extra, a.Inputs...) {
		merged[id] = true
	}
	a.Inputs = sortedKeys(merged)
}
