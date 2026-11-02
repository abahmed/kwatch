package dashboard

// The context parts of the incident page are bounded when they are
// built; these steps cut them further when the detail is still over
// its size limit. Each list is kept most relevant first, so the end
// goes, except the delivery trail, which loses its oldest entries.

// trimChanges drops the last half of the changes.
func trimChanges(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Changes), floor)
	if keep == len(d.Changes) {
		return false
	}
	t.Changes += len(d.Changes) - keep
	d.Changes = d.Changes[:keep]
	return true
}

// trimContainers drops the last (least restarted) half of the rows.
func trimContainers(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	keep := keepCount(len(d.Containers), floor)
	if keep == len(d.Containers) {
		return false
	}
	t.Containers += len(d.Containers) - keep
	d.Containers = d.Containers[:keep]
	return true
}

// trimDeliveries drops the oldest half of the delivery trail.
func trimDeliveries(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	n := len(d.Deliveries)
	keep := keepCount(n, floor)
	if keep == n {
		return false
	}
	t.Deliveries += n - keep
	d.Deliveries = d.Deliveries[n-keep:]
	return true
}

// trimAffected drops the last half of the affected objects; the root
// and the failing ones lead.
func trimAffected(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Affected), floor)
	if keep == len(d.Affected) {
		return false
	}
	t.Affected += len(d.Affected) - keep
	d.Affected = d.Affected[:keep]
	return true
}

// trimOccurrences drops the oldest half of the earlier occurrences
// listed; the counts stay.
func trimOccurrences(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	if d.SeenBefore == nil {
		return false
	}
	recent := d.SeenBefore.Recent
	keep := keepCount(len(recent), floor)
	if keep == len(recent) {
		return false
	}
	t.Occurrences += len(recent) - keep
	d.SeenBefore.Recent = recent[:keep]
	return true
}

// shortenContext cuts the long texts of the context parts with short.
func shortenContext(d *IncidentDetail, short func(*string) bool) {
	for i := range d.Changes {
		shortenChange(&d.Changes[i], short)
	}
	if d.FixedBy != nil {
		shortenChange(d.FixedBy, short)
	}
	for i := range d.Deliveries {
		short(&d.Deliveries[i].Error)
	}
	for i := range d.Containers {
		short(&d.Containers[i].Image)
	}
}

// shortenChange cuts a change's texts and field values with short.
func shortenChange(c *ChangeItem, short func(*string) bool) {
	short(&c.Summary)
	short(&c.Note)
	for j := range c.Fields {
		short(&c.Fields[j].Before)
		short(&c.Fields[j].After)
	}
}
