package compose

// previousFailsSentences say that the version before the latest release
// fails the same way (rootcause.CauseRecord.PreviousFails): the release
// is not blamed and no undo is offered; the reader is pointed at the
// error both versions stop with instead.
func previousFailsSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || !f.p.Cause.PreviousFails {
		return nil
	}
	return []sentence{{part: partChanges, weight: 1,
		text: "The previous version fails the same way: the latest " +
			"release did not break it, the error both versions stop " +
			"with did."}}
}
