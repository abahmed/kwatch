package incident

// sentMark remembers how much of an incident's timeline the last
// delivered message covered. It counts entries instead of comparing
// times: several entries noted in one evaluation share a timestamp, and
// a time mark would either repeat or skip them.
//
// The mark moves only for messages people actually got. A decision that
// is dropped as out of scope, or held back (for an investigation or the
// startup summary), leaves the mark where it was; the hold's release
// moves it.
type sentMark struct {
	// noted counts every entry ever added to the timeline in this
	// process. The timeline keeps only its newest entries, so
	// noted-len(Timeline) entries were trimmed from its front.
	noted int
	// covered is the noted count the last delivered message covered,
	// or unknownMark.
	covered int
	// pending is the noted count of the latest decision, which waits
	// for its delivery to be confirmed or held.
	pending int
	// before is covered from before the latest decision, restored when
	// that decision turns out not to be delivered.
	before int
}

// unknownMark is covered when no one knows what was delivered: after a
// restart the mark is not persisted.
const unknownMark = -1

// restoredMark is the mark of an incident restored with n timeline
// entries: what was delivered before the restart is unknown.
func restoredMark(n int) sentMark {
	return sentMark{noted: n, covered: unknownMark,
		pending: unknownMark, before: unknownMark}
}

// reported converts the mark into a count of the first entries of a
// timeline of length n. known is false when the mark is unknown.
func (s sentMark) reported(n int) (count int, known bool) {
	if s.covered == unknownMark {
		return 0, false
	}
	trimmed := s.noted - n
	return min(max(s.covered-trimmed, 0), n), true
}

// decided records a new decision. delivered is false when the decision
// is known not to reach anyone yet: its incident is out of scope or its
// announcement is held.
func (s *sentMark) decided(delivered bool) {
	s.before, s.pending = s.covered, s.noted
	if delivered {
		s.covered = s.noted
	}
}

// confirm marks the latest decision as delivered.
func (s *sentMark) confirm() {
	s.covered = s.pending
}

// withdraw marks the latest decision as not delivered.
func (s *sentMark) withdraw() {
	s.covered = s.before
}
