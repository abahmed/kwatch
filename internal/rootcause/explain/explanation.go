package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Level words a confidence for readers.
type Level string

// Confidence levels, from ConfidenceFloor up.
const (
	LevelHigh     Level = "high"
	LevelLikely   Level = "likely"
	LevelPossible Level = "possible"
)

// levelOf words a confidence at or above the floor.
func levelOf(confidence float64) Level {
	switch {
	case confidence >= ConfidenceHigh:
		return LevelHigh
	case confidence >= ConfidenceLikely:
		return LevelLikely
	}
	return LevelPossible
}

// Explanation is the result of one solve.
type Explanation struct {
	// Areas are the connected groups of failures, in order of their
	// first failure's key.
	Areas []Area
}

// Area is one connected group of failing entities and its causes.
type Area struct {
	// Failures are the failing entities of the area, sorted.
	Failures []inventory.EntityID
	// Causes are the fewest causes that cover the failures, best
	// first. Empty means the cause is unknown.
	Causes []Cause
	// Unexplained are failures no stated cause covers.
	Unexplained []inventory.EntityID
	// Alternatives are other causes above the floor, best first.
	Alternatives []Cause
	// Unverified names, in plain words, what kwatch could not see where
	// a cause might have been ("secrets in billing"), sorted.
	Unverified []string
	// Inputs are every entity the solve walked through from the
	// failures, sorted. A change to one of them can change the area.
	Inputs []inventory.EntityID
	// Trace is the reasoning behind the choice.
	Trace Trace
}

// Unknown reports whether no cause reached the confidence floor.
func (a Area) Unknown() bool { return len(a.Causes) == 0 }

// Cause is one explanation of some failures.
type Cause struct {
	// Root is the entity blamed.
	Root inventory.EntityID
	// Row names the propagation row that links the root to its
	// failures ("self" when a workload is its own cause).
	Row string
	// Mode is the root's mode that matched the row.
	Mode detection.Mode
	// Covers lists the failures the cause explains, sorted.
	Covers []inventory.EntityID
	// Chain is one path from the root to a failure it explains.
	Chain []inventory.EntityID
	// Summary states the cause in one sentence.
	Summary string
	// Confidence is between 0 and 1.
	Confidence float64
	Level      Level
	// Contributions lists each scorer's contribution, strongest first.
	Contributions []Contribution
	// Changes are the root's changes inside the causal window.
	Changes []inventory.Change
}

// Trace records how an area was solved.
type Trace struct {
	// Candidates are every candidate that was scored, best first.
	Candidates []Cause
	// Rejected are the entities that were considered and dropped.
	Rejected []Rejection
}

// Rejection names an entity that was not chosen and why.
type Rejection struct {
	Root   inventory.EntityID
	Reason string
}

// CauseOf returns the stated cause that covers a failure, and whether
// there is one.
func (e Explanation) CauseOf(id inventory.EntityID) (Cause, bool) {
	for _, area := range e.Areas {
		for _, cause := range area.Causes {
			if containsID(cause.Covers, id) {
				return cause, true
			}
		}
	}
	return Cause{}, false
}

func containsID(ids []inventory.EntityID, id inventory.EntityID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
