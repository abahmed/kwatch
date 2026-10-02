package explain

import "github.com/abahmed/kwatch/internal/inventory"

// maxMergeRounds bounds how often one Solve widens its focus to merge
// with cached areas. Every round removes at least one cached area, so
// the bound only guards against a bug turning into a hang.
const maxMergeRounds = 64

// Solver explains snapshots incrementally. It caches each solved area
// and solves again only the areas a change can affect, so a steady
// cluster costs nothing and a storm is one solve per loop iteration.
//
// An area is stale when one of its inputs is dirty: an input is an
// entity the walk went through upstream of the area's failures, or one
// whose findings decide which links the walk follows (an autoscaler is
// followed only once it fails), so the failures are exactly the
// dependents of that input. A new failure is
// solved on its own first; when one of its candidates is a candidate
// or an input of a cached area, both are solved again together,
// because the areas they form depend on each other.
//
// A Solver is not safe for concurrent use. It does no I/O.
type Solver struct {
	areas []solvedArea
}

// solvedArea is one cached area and the indexes staleness reads.
type solvedArea struct {
	area Area
	// inputs are the area's failures and walked entities.
	inputs map[inventory.EntityID]bool
	// keys are the failures and the candidates the area considered;
	// two areas sharing a key must be solved together.
	keys map[inventory.EntityID]bool
}

// NewSolver builds a solver with an empty cache.
func NewSolver() *Solver { return &Solver{} }

// Solve explains the failures of s that dirty can affect and returns
// the areas it solved again; cached areas that stayed valid are not
// returned. dirty lists the entities whose health, relations, notes or
// changes moved since the last Solve. The snapshot's Focus is ignored.
func (sv *Solver) Solve(s Snapshot, dirty []inventory.EntityID) []Area {
	failing := idSet(failures(s.Findings))
	focus := map[inventory.EntityID]bool{}
	sv.dropStale(idSet(dirty), failing, focus)
	for id := range failing {
		if !sv.covers(id) {
			focus[id] = true
		}
	}
	var fresh []solvedArea
	for round := 0; len(focus) > 0 && round < maxMergeRounds; round++ {
		s.Focus = sortedKeys(focus)
		fresh = solvedAreas(Explain(s))
		if !sv.mergeOverlapping(fresh, failing, focus) {
			break
		}
	}
	sv.areas = append(sv.areas, fresh...)
	out := make([]Area, 0, len(fresh))
	for _, a := range fresh {
		out = append(out, a.area)
	}
	return out
}

// Areas returns every cached area, solved now or earlier.
func (sv *Solver) Areas() []Area {
	out := make([]Area, 0, len(sv.areas))
	for _, a := range sv.areas {
		out = append(out, a.area)
	}
	return out
}

// dropStale removes the areas with a dirty input or a failure that no
// longer fails, and adds their remaining failures to focus.
func (sv *Solver) dropStale(
	dirty, failing, focus map[inventory.EntityID]bool,
) {
	kept := sv.areas[:0]
	for _, a := range sv.areas {
		if !a.stale(dirty, failing) {
			kept = append(kept, a)
			continue
		}
		a.addFailures(failing, focus)
	}
	sv.areas = kept
}

// mergeOverlapping drops the cached areas that share a key with a
// fresh area and adds their failures to focus. It reports whether it
// dropped any, so the caller solves the wider focus again.
func (sv *Solver) mergeOverlapping(
	fresh []solvedArea, failing, focus map[inventory.EntityID]bool,
) bool {
	merged := false
	kept := sv.areas[:0]
	for _, cached := range sv.areas {
		if !overlapsAny(cached, fresh) {
			kept = append(kept, cached)
			continue
		}
		cached.addFailures(failing, focus)
		merged = true
	}
	sv.areas = kept
	return merged
}

// covers reports whether a cached area explains the failure.
func (sv *Solver) covers(id inventory.EntityID) bool {
	for _, a := range sv.areas {
		if containsID(a.area.Failures, id) {
			return true
		}
	}
	return false
}

// stale reports whether a change can affect the cached area.
func (a solvedArea) stale(dirty, failing map[inventory.EntityID]bool) bool {
	for _, id := range a.area.Failures {
		if !failing[id] {
			return true
		}
	}
	for id := range dirty {
		if a.inputs[id] {
			return true
		}
	}
	return false
}

// addFailures adds the area's failures that still fail to focus.
func (a solvedArea) addFailures(
	failing, focus map[inventory.EntityID]bool,
) {
	for _, id := range a.area.Failures {
		if failing[id] {
			focus[id] = true
		}
	}
}

// overlapsAny reports whether a fresh area considered a candidate the
// cached area also considered or walked through: that candidate may now
// explain the cached failures too.
func overlapsAny(cached solvedArea, fresh []solvedArea) bool {
	for _, f := range fresh {
		for id := range f.keys {
			if cached.keys[id] || cached.inputs[id] {
				return true
			}
		}
	}
	return false
}

// solvedAreas indexes the areas of one explanation.
func solvedAreas(e Explanation) []solvedArea {
	out := make([]solvedArea, 0, len(e.Areas))
	for _, area := range e.Areas {
		out = append(out, solvedArea{
			area: area, inputs: idSet(area.Inputs), keys: keysOf(area),
		})
	}
	return out
}

// keysOf lists the failures of an area and every candidate it
// considered, chosen or rejected.
func keysOf(area Area) map[inventory.EntityID]bool {
	keys := idSet(area.Failures)
	for _, c := range area.Trace.Candidates {
		keys[c.Root] = true
	}
	for _, r := range area.Trace.Rejected {
		keys[r.Root] = true
	}
	return keys
}

func idSet(ids []inventory.EntityID) map[inventory.EntityID]bool {
	out := make(map[inventory.EntityID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}
