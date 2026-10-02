package compose

// unclearSentences follow the lead when no cause reached the confidence
// floor although the failure could have one upstream: the root is the
// failing object itself and candidates outside it were weighed and
// dropped. Without a root finding the lead already says no outside
// cause was found, and a cause, however hedged, is a cause.
func unclearSentences(f caseFacts) []sentence {
	p := f.p
	if p.Cause != nil || !p.CauseUnclear ||
		rootFinding(p, f.members) == nil {
		return nil
	}
	return []sentence{{part: partCause, text: "The cause is not clear yet."}}
}
