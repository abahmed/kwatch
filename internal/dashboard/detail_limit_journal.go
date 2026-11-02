package dashboard

// The incident's own record, its ties and what its channels hold are
// bounded where kwatch keeps them; these steps cut them further when
// the detail is still over its size limit. Records kept oldest first
// lose their oldest entries; lists kept most relevant first lose their
// last ones.

// cutFront keeps the newest keep of list (oldest first) and returns it
// with how many it dropped.
func cutFront[T any](list []T, floor int) ([]T, int) {
	keep := keepCount(len(list), floor)
	return list[len(list)-keep:], len(list) - keep
}

// cutBack keeps the first keep of list (most relevant first).
func cutBack[T any](list []T, floor int) ([]T, int) {
	keep := keepCount(len(list), floor)
	return list[:keep], len(list) - keep
}

// trimTransitions drops the oldest half of the state changes.
func trimTransitions(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	var cut int
	d.Transitions, cut = cutFront(d.Transitions, floor)
	t.Transitions += cut
	return cut > 0
}

// trimSnapshots drops the oldest half of the break snapshots; Snapshot
// stays the latest one kept.
func trimSnapshots(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	var cut int
	d.Snapshots, cut = cutFront(d.Snapshots, floor)
	t.Snapshots += cut
	d.Snapshot = nil
	if n := len(d.Snapshots); n > 0 {
		latest := d.Snapshots[n-1]
		d.Snapshot = &latest
	}
	return cut > 0
}

// trimSnapshotRows drops the last half of every snapshot's containers.
func trimSnapshotRows(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	changed := false
	for i := range d.Snapshots {
		var cut int
		d.Snapshots[i].Containers, cut = cutBack(
			d.Snapshots[i].Containers, floor)
		t.SnapshotRows += cut
		changed = changed || cut > 0
	}
	if d.Snapshot != nil {
		d.Snapshot.Containers, _ = cutBack(d.Snapshot.Containers, floor)
	}
	return changed
}

// trimPhases drops the oldest half of the states strip.
func trimPhases(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	var cut int
	d.Phases, cut = cutFront(d.Phases, floor)
	t.Phases += cut
	return cut > 0
}

// trimFixSteps drops the last half of the steps; the first say most.
func trimFixSteps(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	var cut int
	d.FixSteps, cut = cutBack(d.FixSteps, floor)
	t.FixSteps += cut
	return cut > 0
}

// trimRelated drops the last half of the cause and effect incidents.
func trimRelated(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	var by, causing int
	d.CausedBy, by = cutBack(d.CausedBy, floor)
	d.Causing, causing = cutBack(d.Causing, floor)
	t.CausedBy += by
	t.Causing += causing
	return by+causing > 0
}

// trimPager drops the last half of the pagers.
func trimPager(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	var cut int
	d.Pager, cut = cutBack(d.Pager, floor)
	t.Pager += cut
	return cut > 0
}

// trimSent drops the last half of the sent texts.
func trimSent(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	var cut int
	d.Sent, cut = cutBack(d.Sent, floor)
	t.Sent += cut
	return cut > 0
}

// trimMovedFrom drops the earliest half of the incidents moved in.
func trimMovedFrom(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	if d.Moved == nil {
		return false
	}
	var cut int
	d.Moved.From, cut = cutFront(d.Moved.From, floor)
	t.MovedFrom += cut
	if d.Moved.Into == nil && len(d.Moved.From) == 0 {
		d.Moved = nil
	}
	return cut > 0
}

// trimChangedBefore drops the older half of the changes made before
// the recovery (kept newest first).
func trimChangedBefore(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	var cut int
	d.ChangedBeforeRecovery, cut = cutBack(d.ChangedBeforeRecovery, floor)
	t.ChangedBeforeRecovery += cut
	return cut > 0
}

// shortenJournal cuts the long texts of the record, the ties and the
// sent texts with short; a sent text cut here is marked Cut.
func shortenJournal(d *IncidentDetail, short func(*string) bool) {
	for _, s := range []*string{&d.Title, &d.RootCause, &d.BlastRadius,
		&d.Fix} {
		short(s)
	}
	if d.Why != nil {
		short(&d.Why.AlertLevelReason)
		short(&d.Why.StillOpenReason)
	}
	if d.Ack != nil {
		short(&d.Ack.Note)
	}
	for i := range d.FixSteps {
		short(&d.FixSteps[i].Text)
	}
	for _, list := range [][]RelatedIncident{d.CausedBy, d.Causing} {
		for i := range list {
			short(&list[i].Title)
		}
	}
	for i := range d.Sent {
		if short(&d.Sent[i].Text) {
			d.Sent[i].Cut = true
		}
	}
	for i := range d.Pager {
		short(&d.Pager[i].Reason)
	}
	for i := range d.ChangedBeforeRecovery {
		short(&d.ChangedBeforeRecovery[i].Summary)
		short(&d.ChangedBeforeRecovery[i].Note)
	}
	if d.Moved != nil {
		if d.Moved.Into != nil {
			short(&d.Moved.Into.Title)
		}
		for i := range d.Moved.From {
			short(&d.Moved.From[i].Title)
		}
	}
}
