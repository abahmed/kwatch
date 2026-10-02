package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// BlamedChanges returns the changes of c that can have caused its
// failures, oldest first. A change made after the first failure began
// cannot have caused it, and a scale-only change is usually a reaction:
// an autoscaler adding replicas after a bad rollout. Scale changes are
// kept only when nothing else changed before the failure, because a
// scale-up can be the cause (no room for the new replicas).
func (s Snapshot) BlamedChanges(c Cause) []inventory.Change {
	onset := s.blameOnset(c)
	var before, scales []inventory.Change
	for _, change := range c.Changes {
		if !onset.IsZero() && change.At.After(onset) {
			continue
		}
		if change.Classify() == inventory.ClassScale {
			scales = append(scales, change)
			continue
		}
		before = append(before, change)
	}
	if len(before) == 0 {
		return scales
	}
	return before
}

// blameOnset is when the failure of c began.
//
// The failure starts with the root's own findings when it has any (a
// node under memory pressure), otherwise with the first failure it
// explains (the crashing pods of a rollout). Covered failures that
// began earlier for another reason must not hide the root's change.
//
// Of the root's findings, those that began after its last healthy mark
// count: an older one belongs to an earlier episode. When even those
// predate every change of c, the root was already unhealthy before
// anything changed (a Deployment degraded for days), and that old state
// says nothing about a recent rollout; the covered failures' onset is
// used instead.
func (s Snapshot) blameOnset(c Cause) time.Time {
	root := []inventory.EntityID{c.Root}
	onset := s.onset(root, s.lastHealthy(c.Root))
	if onset.IsZero() {
		onset = s.onset(root, time.Time{})
	}
	if !onset.IsZero() && !predatesAll(onset, c.Changes) {
		return onset
	}
	if covered := s.onset(c.Covers, time.Time{}); !covered.IsZero() {
		return covered
	}
	return onset
}

// healthMarks is the part of the model that keeps health marks. A
// Model reader without them, as in some fixtures, has no healthy mark.
type healthMarks interface {
	HealthMarks(id inventory.EntityID, since time.Time) []inventory.HealthMark
}

// lastHealthy is the time of id's latest healthy mark, or zero.
func (s Snapshot) lastHealthy(id inventory.EntityID) time.Time {
	marks, ok := s.Model.(healthMarks)
	if !ok {
		return time.Time{}
	}
	var last time.Time
	for _, mark := range marks.HealthMarks(id, time.Time{}) {
		if !mark.Failing && mark.At.After(last) {
			last = mark.At
		}
	}
	return last
}

// predatesAll reports whether at is before every change.
func predatesAll(at time.Time, changes []inventory.Change) bool {
	for _, change := range changes {
		if !at.Before(change.At) {
			return false
		}
	}
	return true
}

// onset is when the earliest unhealthy finding of the covered failures
// that began at or after since began, or zero when none is known.
func (s Snapshot) onset(
	covers []inventory.EntityID, since time.Time,
) time.Time {
	var first time.Time
	for _, id := range covers {
		for _, f := range s.Findings[id] {
			if !unhealthy(f) || f.Since.IsZero() || f.Since.Before(since) {
				continue
			}
			if first.IsZero() || f.Since.Before(first) {
				first = f.Since
			}
		}
	}
	return first
}
