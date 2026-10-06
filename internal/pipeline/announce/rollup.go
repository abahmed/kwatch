package announce

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// rollupMin is how many announcements made in the same tick are sent as
// one roll-up message instead of one message each. Problems that start
// at the same moment usually belong together, and seven messages in one
// minute read as noise even when each is right.
const rollupMin = 2

// CollectRollup sends the announcements of one tick as one roll-up when
// there are rollupMin or more, and returns the other decisions. Like the
// startup summary, the roll-up only names the incidents: each keeps its
// own conversation, its first own message introduces it in full, and
// page-tier announcements still go on their own to the paging tools,
// which never receive summaries. It reports whether a roll-up was sent.
func (c *Collector) CollectRollup(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	var announcements, rest []incident.Decision
	for _, d := range decisions {
		if d.Action == incident.Announce {
			announcements = append(announcements, d)
		} else {
			rest = append(rest, d)
		}
	}
	if len(announcements) < rollupMin {
		return decisions, false
	}
	msg := c.env.Messages.Rollup(announcements, now)
	c.sendListing(ctx, now, announcements, msg, false)
	return rest, true
}

// sendListing sends msg, a roll-up or a namespace outage naming the
// announcements, and remembers what it listed. The members' own later
// messages thread under it, and the listing closes once all of them
// resolved. Page-tier members still go on their own to the paging tools,
// which never receive summaries: every one of them, or with pageOnce just
// the first, which then stands for the whole listing.
func (c *Collector) sendListing(
	ctx context.Context, now time.Time, announcements []incident.Decision,
	msg notification.Message, pageOnce bool,
) {
	for _, d := range announcements {
		// Persisted as not announced until delivery has the listing.
		c.env.Incidents.HoldAnnouncement(d.Incident.ID)
		c.env.Incidents.RecordRolledUp(d.Incident.ID)
		msg.Members = append(msg.Members, d.Incident.ID)
	}
	c.env.Sink(ctx, incident.Decision{Reason: "roll-up"}, msg)
	paged := pageOnce && c.anyPaged(announcements)
	for _, d := range announcements {
		if d.Incident.Tier == incident.Page && !(pageOnce && paged) &&
			!c.pagedAlready(d) {
			c.pageOnly(ctx, now, d)
			paged = true
			continue
		}
		c.recordCarried(ctx, now, d, "roll-up")
	}
	s := &c.Startup.Summary
	s.Rollups = append(s.Rollups, c.listed(msg.Key, announcements))
	c.env.SaveStartup(*s)
}

// pagedAlready reports a page-tier announcement whose alert is already
// open at the pagers: an outage hold paged it (PagedAlready) or an earlier
// message did. The listing only names it; paging it again would tell the
// pagers the same thing twice.
func (c *Collector) pagedAlready(d incident.Decision) bool {
	return d.PagedAlready || c.env.Incidents.PageOpen(d.Incident.ID)
}

// anyPaged reports whether one of the announcements already paged.
func (c *Collector) anyPaged(announcements []incident.Decision) bool {
	for _, d := range announcements {
		if d.Incident.Tier == incident.Page && c.env.Incidents.Paged(d.Incident.ID) {
			return true
		}
	}
	return false
}
