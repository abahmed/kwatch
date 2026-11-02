package dashboard

// The insight parts of the incident page are bounded when they are
// built; these steps cut them further when the detail is still over
// its size limit. Each list is kept most relevant first, so the end
// goes.

// trimWarnings drops the oldest half of the recent warnings.
func trimWarnings(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Warnings), floor)
	if keep == len(d.Warnings) {
		return false
	}
	t.Warnings += len(d.Warnings) - keep
	d.Warnings = d.Warnings[:keep]
	return true
}

// trimTraffic drops the last half of the traffic rows; those with
// endpoints not ready lead.
func trimTraffic(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Traffic), floor)
	if keep == len(d.Traffic) {
		return false
	}
	t.Traffic += len(d.Traffic) - keep
	d.Traffic = d.Traffic[:keep]
	return true
}

// trimNodes drops the last half of the nodes; the root and unhealthy
// nodes lead.
func trimNodes(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Nodes), floor)
	if keep == len(d.Nodes) {
		return false
	}
	t.Nodes += len(d.Nodes) - keep
	d.Nodes = d.Nodes[:keep]
	return true
}

// trimProbes drops the last half of the failing probes.
func trimProbes(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.Probes), floor)
	if keep == len(d.Probes) {
		return false
	}
	t.Probes += len(d.Probes) - keep
	d.Probes = d.Probes[:keep]
	return true
}

// trimWaitReasons drops the last half of the scheduler's reasons;
// those ruling out most nodes lead. The raw message stays.
func trimWaitReasons(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	if d.Waiting == nil {
		return false
	}
	reasons := d.Waiting.Reasons
	keep := keepCount(len(reasons), floor)
	if keep == len(reasons) {
		return false
	}
	t.WaitReasons += len(reasons) - keep
	d.Waiting.Reasons = reasons[:keep]
	return true
}

// trimUsageHours drops the last half of the usage histories; the most
// restarted containers lead.
func trimUsageHours(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	keep := keepCount(len(d.UsageHours), floor)
	if keep == len(d.UsageHours) {
		return false
	}
	t.UsageHours += len(d.UsageHours) - keep
	d.UsageHours = d.UsageHours[:keep]
	return true
}

// trimSameTime drops the last half of the issues that started at the
// same time; the closest relations lead.
func trimSameTime(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	keep := keepCount(len(d.SameTime), floor)
	if keep == len(d.SameTime) {
		return false
	}
	t.SameTime += len(d.SameTime) - keep
	d.SameTime = d.SameTime[:keep]
	return true
}

// shortenInsight cuts the long texts of the insight parts with short.
func shortenInsight(d *IncidentDetail, short func(*string) bool) {
	for i := range d.Warnings {
		short(&d.Warnings[i].Message)
	}
	for i := range d.Nodes {
		for j := range d.Nodes[i].Conditions {
			short(&d.Nodes[i].Conditions[j].Message)
		}
	}
	for i := range d.Probes {
		short(&d.Probes[i].LastMessage)
	}
	if d.Waiting != nil {
		short(&d.Waiting.Message)
	}
	for i := range d.SameTime {
		short(&d.SameTime[i].Title)
	}
}
