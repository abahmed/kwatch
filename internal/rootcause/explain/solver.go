package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Explain solves a snapshot: it groups the failures into connected
// areas and, for each, chooses the fewest causes that cover them with
// a greedy weighted set cover. It is a pure function of the snapshot.
func Explain(s Snapshot) Explanation {
	v := newView(s)
	all := focused(failures(s.Findings), s.Focus)
	ranked := v.rank(v.candidates(all))
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].raw > ranked[j].raw
	})
	viable, rejected := splitViable(ranked, v.rejected)
	var out Explanation
	for _, area := range areasOf(all, viable) {
		out.Areas = append(out.Areas, v.solveArea(area, ranked, viable,
			rejected))
	}
	return out
}

// rank scores every candidate. Candidates that blame something other
// than the failures' own workloads go first: a workload is only its own
// cause when none of them explains its failures, and the self scorers
// read which failures they explained.
func (v *view) rank(cs *candidateSet) []scored {
	v.rejected = cs.rejected
	v.explainedBy = map[inventory.EntityID]bool{}
	var ranked, selves []scored
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		if c.isSelf() {
			continue
		}
		s := v.score(c)
		ranked = append(ranked, s)
		if s.veto == "" && s.confidence >= ConfidenceFloor {
			for _, effect := range c.others() {
				v.explainedBy[effect] = true
			}
		}
	}
	for _, id := range sortedKeys(cs.byID) {
		if c := cs.byID[id]; c.isSelf() {
			selves = append(selves, v.score(c))
		}
	}
	return append(ranked, selves...)
}

// splitViable keeps candidates above the floor and not vetoed, and
// adds the others to the rejections with their reason.
func splitViable(
	ranked []scored, rejected map[inventory.EntityID]rejectNote,
) ([]scored, map[inventory.EntityID]rejectNote) {
	var viable []scored
	for _, s := range ranked {
		switch {
		case s.veto != "":
			rejected[s.c.id] = rejectNote{reason: s.veto}
		case s.confidence < ConfidenceFloor:
			rejected[s.c.id] = rejectNote{
				reason: "confidence below the floor"}
		default:
			viable = append(viable, s)
			delete(rejected, s.c.id)
		}
	}
	return viable, rejected
}

// solveArea runs the set cover over one area.
func (v *view) solveArea(
	failures []inventory.EntityID, ranked, viable []scored,
	rejected map[inventory.EntityID]rejectNote,
) Area {
	area := Area{Failures: failures}
	inArea := map[inventory.EntityID]bool{}
	for _, f := range failures {
		inArea[f] = true
	}
	chosen, uncovered := setCover(inArea, viable)
	isChosen := map[inventory.EntityID]bool{}
	for _, s := range chosen {
		isChosen[s.c.id] = true
		area.Causes = append(area.Causes, v.causeOf(s))
	}
	area.Unexplained = uncovered
	for _, s := range viable {
		if !isChosen[s.c.id] && touches(s, inArea) &&
			len(area.Alternatives) < MaxAlternatives {
			area.Alternatives = append(area.Alternatives, v.causeOf(s))
		}
	}
	area.Trace = v.traceOf(ranked, inArea, rejected, isChosen)
	area.Unverified, area.Inputs = v.areaInputs(failures)
	return area
}

// areaInputs merges the unverified notes and walked entities of an
// area's failures.
func (v *view) areaInputs(
	failures []inventory.EntityID,
) ([]string, []inventory.EntityID) {
	var notes []string
	inputs := map[inventory.EntityID]bool{}
	for _, f := range failures {
		notes = append(notes, v.unverified[f]...)
		for _, id := range v.inputs[f] {
			inputs[id] = true
		}
	}
	return rootcause.MergeUnverified(notes), sortedKeys(inputs)
}

// focused keeps the failures a snapshot's Focus names; a nil Focus
// keeps them all.
func focused(
	all, focus []inventory.EntityID,
) []inventory.EntityID {
	if focus == nil {
		return all
	}
	wanted := map[inventory.EntityID]bool{}
	for _, id := range focus {
		wanted[id] = true
	}
	var out []inventory.EntityID
	for _, id := range all {
		if wanted[id] {
			out = append(out, id)
		}
	}
	return out
}

// setCover picks, while failures remain uncovered, the candidate that
// covers the most of them weighted by its score. Ties go to the higher
// score, then to the smaller key, so the result never
// depends on map order.
func setCover(
	inArea map[inventory.EntityID]bool, viable []scored,
) ([]scored, []inventory.EntityID) {
	uncovered := map[inventory.EntityID]bool{}
	for id := range inArea {
		uncovered[id] = true
	}
	var chosen []scored
	for len(uncovered) > 0 {
		best, gain := -1, 0.0
		for i, s := range viable {
			g := float64(countCovered(s, uncovered)) * s.raw
			if g > gain || (g == gain && best >= 0 && g > 0 &&
				better(s, viable[best])) {
				best, gain = i, g
			}
		}
		if best < 0 {
			break
		}
		chosen = append(chosen, viable[best])
		for effect := range viable[best].c.covers {
			delete(uncovered, effect)
		}
	}
	return chosen, sortedKeys(uncovered)
}

// better breaks set-cover ties.
func better(a, b scored) bool {
	if a.raw != b.raw {
		return a.raw > b.raw
	}
	return a.c.id.String() < b.c.id.String()
}

func countCovered(s scored, uncovered map[inventory.EntityID]bool) int {
	n := 0
	for effect := range s.c.covers {
		if uncovered[effect] {
			n++
		}
	}
	return n
}

// touches reports whether a candidate covers a failure of the area.
func touches(s scored, inArea map[inventory.EntityID]bool) bool {
	for effect := range s.c.covers {
		if inArea[effect] {
			return true
		}
	}
	return false
}

// sortedKeys returns a map's keys by canonical key.
func sortedKeys[T any](m map[inventory.EntityID]T) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sortIDs(out)
	return out
}
