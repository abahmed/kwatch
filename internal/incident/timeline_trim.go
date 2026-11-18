package incident

import (
	"slices"
	"strings"
)

// Timeline entries kept whatever else is dropped: the first timelineHead
// say how the incident began, the newest timelineTail say how it stands.
const (
	timelineHead = 8
	timelineTail = 24
)

// trimTimeline drops n entries to keep the timeline within maxTimeline.
// It drops repeats first: the oldest entry that a later entry says
// again, about the same member, folds into that later entry's Repeats.
// A steady failure that keeps coming back then cannot push the rest of
// the story (when it started, what changed the cause) out of the
// timeline. Without repeats it thins the middle: the first entries and
// the newest ones stay, so a long-lived incident still shows how it
// began and where it is now, and each dropped entry is counted in the
// Gap of the entry before it. Among the middle ones those that mark a
// phase or a decision (marksPhase) go last.
//
// Only entries the last delivered message covered are folded. A dropped
// middle entry may be one no message covered yet (entries pile up while
// an update is held), so every removal tells sentMark where it was
// (sentMark.removed) and the count of reported entries stays exact: the
// next update neither repeats a line already sent nor skips one. An
// unknown mark (after a restart) covers none and leaves the choice free.
func (inc *Incident) trimTimeline(n int) {
	for ; n > 0; n-- {
		i, twin := inc.foldableRepeat()
		if i < 0 {
			inc.dropFromMiddle()
			continue
		}
		inc.Timeline[twin].Repeats += inc.Timeline[i].Repeats + 1
		inc.removeEntry(i)
	}
}

// dropFromMiddle drops one entry from the middle of the timeline and
// counts it, with all it stood for, in the Gap of the entry before it.
func (inc *Incident) dropFromMiddle() {
	at := inc.thinnableEntry()
	gone := inc.Timeline[at]
	inc.removeEntry(at)
	inc.Timeline[at-1].Gap += 1 + gone.Repeats + gone.Gap
}

// removeEntry deletes the entry at index at and moves the sent mark
// with it.
func (inc *Incident) removeEntry(at int) {
	inc.sent.removed(inc.sent.noted - len(inc.Timeline) + at + 1)
	inc.Timeline = slices.Delete(slices.Clone(inc.Timeline), at, at+1)
}

// thinnableEntry is the index of the entry to drop from the middle of
// the timeline: the oldest one that marks no phase, else the oldest of
// the middle.
func (inc *Incident) thinnableEntry() int {
	from := max(1, min(timelineHead, len(inc.Timeline)-1))
	to := max(from+1, len(inc.Timeline)-timelineTail)
	for i := from; i < to && i < len(inc.Timeline); i++ {
		if !marksPhase(inc.Timeline[i]) {
			return i
		}
	}
	return from
}

// marksPhase reports an entry that tells a phase or decision change of
// the incident (its cause revised, told again, flapping, resolved,
// reopened), as opposed to a member joining, recovering or failing
// again.
func marksPhase(e Event) bool {
	return e.Entity == nil && !strings.HasPrefix(e.Text, "failing again: ")
}

// foldableRepeat returns the oldest entry that may be folded and its
// later twin, or -1 when there is none.
func (inc *Incident) foldableRepeat() (int, int) {
	limit := len(inc.Timeline) - 1
	if reported, known := inc.sent.reported(len(inc.Timeline)); known {
		limit = min(limit, reported)
	}
	for i := 0; i < limit; i++ {
		for j := i + 1; j < len(inc.Timeline); j++ {
			if sameEvent(inc.Timeline[i], inc.Timeline[j]) {
				return i, j
			}
		}
	}
	return -1, -1
}

// sameEvent reports whether two entries say the same thing about the
// same member, whenever they were noted.
func sameEvent(a, b Event) bool {
	if a.Text != b.Text || (a.Entity == nil) != (b.Entity == nil) {
		return false
	}
	return a.Entity == nil || *a.Entity == *b.Entity
}
