package dashboard

// The summary is what a reader sees first, so its stages come last:
// everything else is cut before it is.

// trimSummaryTold drops the last half of the channels told.
func trimSummaryTold(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	if d.Summary == nil {
		return false
	}
	var cut int
	d.Summary.Told, cut = cutBack(d.Summary.Told, floor)
	t.SummaryTold += cut
	return cut > 0
}

// trimSummaryProof drops the last half of the proof items: they are
// listed strongest first.
func trimSummaryProof(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	if d.Summary == nil {
		return false
	}
	var cut int
	d.Summary.Proof, cut = cutBack(d.Summary.Proof, floor)
	t.SummaryProof += cut
	return cut > 0
}

// trimSummaryNext drops the first step, whose command is never cut.
func trimSummaryNext(d *IncidentDetail, t *DetailTruncation, _ int) bool {
	if d.Summary == nil || d.Summary.Next == nil {
		return false
	}
	d.Summary.Next = nil
	t.SummaryNext++
	return true
}

// shortenSummary cuts the summary's sentences and proof values to
// limit runes; the step's command is never cut.
func shortenSummary(
	d *IncidentDetail, t *DetailTruncation, limit int,
) bool {
	s := d.Summary
	if s == nil {
		return false
	}
	n := 0
	short := func(v *string) {
		if cut, ok := shortText(*v, limit); ok {
			*v = cut
			n++
		}
	}
	short(&s.What)
	short(&s.Why.Text)
	for i := range s.Proof {
		short(&s.Proof[i].Text)
		short(&s.Proof[i].Detail)
	}
	if s.Next != nil {
		short(&s.Next.Text)
	}
	t.Texts += n
	return n > 0
}
