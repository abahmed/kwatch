package pipeline

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// rollupMin is how many announcements made in the same tick are sent as
// one roll-up message instead of one message each. Problems that start
// at the same moment usually belong together, and seven messages in one
// minute read as noise even when each is right.
const rollupMin = 2

// collectRollup sends the announcements of one tick as one roll-up when
// there are rollupMin or more, and returns the other decisions. Like the
// startup summary, the roll-up only names the incidents: each keeps its
// own conversation, its first own message introduces it in full, and
// page-tier announcements still go on their own to the paging tools,
// which never receive summaries. It reports whether a roll-up was sent.
func (a *announcer) collectRollup(
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
	for _, d := range announcements {
		// Persisted as not announced until delivery has the roll-up.
		a.incidents.HoldAnnouncement(d.Incident.ID)
	}
	msg := a.messages.Rollup(announcements, now)
	a.sink(ctx, incident.Decision{Reason: "roll-up"}, msg)
	for _, d := range announcements {
		if d.Incident.Tier == incident.Page {
			paging := a.write(d, now)
			paging.PagingOnly = true
			a.sink(ctx, d, paging)
			continue
		}
		a.recordCarried(ctx, now, d, "roll-up")
	}
	s := &a.startup.summary
	s.Rollups = append(s.Rollups, a.listed(msg.Key, announcements))
	a.storage.saveStartup(*s)
	return rest, true
}
