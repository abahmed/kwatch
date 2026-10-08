package explain

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Narrowing called-service candidates by workload count and time window.

// applyMinWorkloads drops rows whose effects belong to fewer distinct
// workloads than the row needs. Candidates, rows and effects are
// visited in order so the first reject note is the same on every run.
func (v *view) applyMinWorkloads(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		for _, group := range effectsNeedingWorkloads(c) {
			counted := group.effects
			if group.inferred {
				// Any other failure the cause explains, such as its
				// own servers crashing, corroborates what the text
				// says.
				counted = c.direct()
			}
			if v.workloadsOf(counted) >= group.need {
				continue
			}
			for _, effect := range group.effects {
				delete(c.covers, effect)
				cs.rejectInsufficient(c.id, effect, "row "+group.row+
					" needs failures in more workloads")
			}
		}
		if len(c.direct()) == 0 {
			delete(cs.byID, c.id)
		}
	}
}

// workloadGroup is the effects one row covers for a candidate and the
// workloads they must span.
type workloadGroup struct {
	row  string
	need int
	// inferred marks a need raised by InferredMinWorkloads.
	inferred bool
	effects  []inventory.EntityID
}

// effectsNeedingWorkloads groups a candidate's direct effects by the
// row that covers them, for the rows that need several workloads,
// sorted by row and need.
func effectsNeedingWorkloads(c *candidate) []workloadGroup {
	type groupKey struct {
		row      string
		need     int
		inferred bool
	}
	index := map[groupKey]int{}
	var out []workloadGroup
	for _, effect := range c.direct() {
		how := c.covers[effect]
		need := how.match.minWorkloads()
		if need == 0 {
			continue
		}
		key := groupKey{row: how.match.row.Name, need: need,
			inferred: need != how.match.row.MinWorkloads}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, workloadGroup{row: key.row, need: need,
				inferred: key.inferred})
		}
		out[i].effects = append(out[i].effects, effect)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].row != out[j].row {
			return out[i].row < out[j].row
		}
		return out[i].need < out[j].need
	})
	return out
}

// applySignatureWindow keeps failure-signature coverage only for the
// failures that began within SignatureWindow of each other. Why: the
// same words weeks apart are two outages, not one; a shared error that
// takes down many workloads does so within minutes. The densest
// window of start times is kept; effects with no known start stay.
func (v *view) applySignatureWindow(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		if c.id.Kind != KindFailureSignature {
			continue
		}
		effects := c.direct()
		starts := map[inventory.EntityID]time.Time{}
		var timed []inventory.EntityID
		for _, effect := range effects {
			start := v.earliestSince([]inventory.EntityID{effect})
			if start.ok {
				starts[effect] = start.t
				timed = append(timed, effect)
			}
		}
		sort.SliceStable(timed, func(i, j int) bool {
			return starts[timed[i]].Before(starts[timed[j]])
		})
		keep := v.densestWindow(timed, starts)
		for _, effect := range timed {
			if keep[effect] {
				continue
			}
			delete(c.covers, effect)
			cs.rejectInsufficient(c.id, effect, "the failures sharing "+
				"this error did not begin together")
		}
		if len(c.direct()) == 0 {
			delete(cs.byID, c.id)
		}
	}
}

// densestWindow returns the effects, sorted by start, inside the
// SignatureWindow that holds failures of the most distinct workloads:
// one workload with many failing replicas must not outvote several
// workloads that failed together. The number of effects breaks a tie,
// then the earliest window. The window slides over the sorted effects
// once, counting how many of its effects each workload has.
func (v *view) densestWindow(
	sorted []inventory.EntityID, starts map[inventory.EntityID]time.Time,
) map[inventory.EntityID]bool {
	owners := make([]inventory.EntityID, len(sorted))
	for i, effect := range sorted {
		owners[i] = v.workloadOf(effect)
	}
	inWindow := map[inventory.EntityID]int{}
	bestFrom, bestTo, bestWorkloads := 0, -1, 0
	to := -1
	for from := range sorted {
		for to+1 < len(sorted) && !starts[sorted[to+1]].After(
			starts[sorted[from]].Add(SignatureWindow)) {
			to++
			inWindow[owners[to]]++
		}
		if len(inWindow) > bestWorkloads || (len(inWindow) ==
			bestWorkloads && to-from > bestTo-bestFrom) {
			bestFrom, bestTo, bestWorkloads = from, to, len(inWindow)
		}
		// The effect at from leaves before the next window starts.
		inWindow[owners[from]]--
		if inWindow[owners[from]] == 0 {
			delete(inWindow, owners[from])
		}
	}
	keep := map[inventory.EntityID]bool{}
	for i := bestFrom; i <= bestTo; i++ {
		keep[sorted[i]] = true
	}
	return keep
}
