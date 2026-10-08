package announce

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// restoredSummary names the carrier of a decision the restored-incidents
// summary takes in, for the audit log.
const restoredSummary = "restored summary"

// maxFolded bounds the memory of folded demotions; past it the next
// restored incident that falls to the digest tier may post once more.
const maxFolded = 4096

// carriesThread reports a decision of an incident that fell to the digest
// tier that belongs to its thread (see threadNews). The first one of an
// incident restored from an earlier run does not: right after a restart
// its thread may never have been told, and a post for each of hundreds of
// restored incidents is a burst. The restored-incidents summary and the
// next digest list it as still failing instead.
func (c *Collector) carriesThread(d incident.Decision) bool {
	if !threadNews(d) {
		return false
	}
	id := d.Incident.ID
	if d.Action != incident.Update || c.folded[id] ||
		!c.env.Incidents.Restored(id) {
		return true
	}
	if len(c.folded) >= maxFolded {
		c.folded = map[string]bool{}
	}
	c.folded[id] = true
	return false
}

// holdWarm takes in the announcements of problems found after a restart
// that have been failing since long before it: kwatch only just looked,
// so they are not news of their own, and the restored-incidents summary
// names them like their siblings. It only holds in the restore grace,
// before that summary goes out. Updates and resolves of a held problem
// fold into the held entry.
func (c *Collector) holdWarm(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) []incident.Decision {
	s := &c.Startup
	if s.WarmAt.IsZero() || !now.Before(s.WarmAt) {
		return decisions
	}
	var rest []incident.Decision
	for _, d := range decisions {
		if !c.holdWarmOne(ctx, now, d) {
			rest = append(rest, d)
		}
	}
	return rest
}

// holdWarmOne reports whether d was absorbed by the restored summary.
func (c *Collector) holdWarmOne(
	ctx context.Context, now time.Time, d incident.Decision,
) bool {
	s := &c.Startup
	id := d.Incident.ID
	held := indexOfIncident(s.Warm, id)
	switch {
	case held < 0 && d.Action == incident.Announce &&
		d.Incident.Tier == incident.Notify && c.predatesBoot(d.Incident, now):
		s.Warm = append(s.Warm, d)
		c.env.Incidents.HoldAnnouncement(id)
		c.env.Incidents.RecordRolledUp(id)
		c.recordCarried(ctx, now, d, restoredSummary)
		return true
	case held >= 0 && d.Action == incident.Update:
		s.Warm[held].Incident = d.Incident
		c.recordCarried(ctx, now, d, restoredSummary)
		return true
	case held >= 0 && d.Action == incident.Resolve:
		s.Warm = append(s.Warm[:held], s.Warm[held+1:]...)
		c.closeUnannounced(ctx, now, d, restoredSummary)
		return true
	}
	return false
}

// predatesBoot reports a problem whose findings began well before kwatch
// started: its earliest finding is older than the startup window. One
// without a known start is treated as new.
func (c *Collector) predatesBoot(p incident.Incident, now time.Time) bool {
	boot := c.seenAt
	if boot.IsZero() {
		boot = now
	}
	var first time.Time
	for _, m := range p.Members {
		if !m.Advisory && !m.Since.IsZero() &&
			(first.IsZero() || m.Since.Before(first)) {
			first = m.Since
		}
	}
	return !first.IsZero() && first.Before(boot.Add(-StartupWindow))
}

// takeWarm returns the problems the restored summary takes in, as
// decisions for it, and releases their held announcements: the summary
// is their message.
func (c *Collector) takeWarm(listed []incident.Decision) []incident.Decision {
	s := &c.Startup
	for _, d := range s.Warm {
		if indexOfIncident(listed, d.Incident.ID) < 0 {
			d.Reason = "restored"
			listed = append(listed, d)
		}
		c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
	}
	s.Warm = nil
	return listed
}
