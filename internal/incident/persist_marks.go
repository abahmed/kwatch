package incident

import "time"

// Marks are small facts about an incident that must survive a restart.
// They are saved with its record (Record embeds them, so each is a
// field of the saved record) and restored as they were. A record from
// before a mark existed restores without it.
type Marks struct {
	// ScaleHeldUntil is when the scaling hold of a settling incident
	// ends (scale_hold.go). The wave that held it is not saved, so
	// after a restart this is all that keeps the hold: without it the
	// held page would go out at once. Zero when it is not held.
	ScaleHeldUntil time.Time `json:",omitzero"`
	// Rose says a finding of the incident was low and old, then got
	// worse (old_problem.go). The incident is not lowered for its age
	// again, in this run or the next.
	Rose bool `json:",omitempty"`
	// PagedBelowPage and PageRaised track a page that rose after the
	// pagers heard the incident below the page tier (page_raise.go).
	PagedBelowPage bool `json:",omitempty"`
	PageRaised     bool `json:",omitempty"`
}
