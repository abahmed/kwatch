package incident

import "slices"

// trimTimeline drops n entries to keep the timeline within maxTimeline.
// It drops repeats first: the oldest entry that a later entry says
// again, about the same member, folds into that later entry's Repeats.
// A steady failure that keeps coming back then cannot push the rest of
// the story (when it started, what changed the cause) out of the
// timeline. Without repeats it drops the oldest entries, as before.
//
// Only entries the last delivered message covered are folded: sentMark
// counts trimmed entries as if they left the front, which holds for any
// entry before its mark. An unknown mark (after a restart) covers none
// and leaves the choice free.
func (inc *Incident) trimTimeline(n int) {
	for ; n > 0; n-- {
		i, twin := inc.foldableRepeat()
		if i < 0 {
			inc.Timeline = append(inc.Timeline[:0:0], inc.Timeline[n:]...)
			return
		}
		inc.Timeline[twin].Repeats += inc.Timeline[i].Repeats + 1
		inc.Timeline = slices.Delete(slices.Clone(inc.Timeline), i, i+1)
	}
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
