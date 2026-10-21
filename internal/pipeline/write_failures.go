package pipeline

import "k8s.io/klog/v2"

// writeLogEvery is how many failing batches pass between two log lines
// about a store that stays down. The writers retry every writeBatch, so
// a failing store is a line every half minute, not one per second.
const writeLogEvery = 30

// writeStreak counts the consecutive batches a writer failed. Only the
// writer's goroutine touches it.
type writeStreak struct{ batches int }

// log reports a failed write of what: for the first failing batch, then
// for every writeLogEvery-th one.
func (s *writeStreak) log(err error, what string) {
	if s.batches%writeLogEvery != 0 {
		return
	}
	klog.ErrorS(err, "pipeline: storage write failed; retrying next batch",
		"component", "pipeline", "operation", "save", "data", what,
		"failedBatches", s.batches+1)
}

// end closes a batch: a failed one extends the streak, a good one ends it
// and says that the store recovered.
func (s *writeStreak) end(failed bool) {
	switch {
	case failed:
		s.batches++
	case s.batches > 0:
		klog.InfoS("pipeline: storage writes recovered",
			"component", "pipeline", "operation", "save",
			"failedBatches", s.batches)
		s.batches = 0
	}
}
