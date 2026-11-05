package announce

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ListRestored sends the message naming the incidents restored from the
// previous session that still fail: a restart announces none of them, so
// without it nobody would hear that they are still open. The findings of
// a restored incident come back within the restore grace, so the message
// goes out when the grace is over (WarmAt), so a restart sends one
// message. Only an urgent incident that is already back (page tier, its
// findings still news) does not wait for the rest: it is listed one
// settle after the sources synced, with the others that are back by
// then. Whatever comes back later is listed when the grace is over, if it
// is not named already. An incident a listing named
// within RestoredRelist is not listed again: when nothing else is, one
// short restart note says how many are still open (restart_note.go). It
// reports whether it sent a message.
func (c *Collector) ListRestored(ctx context.Context, now time.Time) bool {
	s := &c.Startup
	if s.WarmAt.IsZero() {
		return c.listLateRestored(ctx, now)
	}
	final := !now.Before(s.WarmAt)
	if !final && !c.earlyListingDue(now) {
		return false
	}
	s.earlyDone = true
	listed, repeated, lower := c.restoredNews(now, final)
	listed = c.takeWarm(listed)
	if final {
		// Findings held by a wake-up or a fresh pod's grace come back
		// after the grace: keep watching for them (listLateRestored).
		s.lateUntil = s.WarmAt.Add(kube.WakeMax)
		s.WarmAt = time.Time{}
	}
	if len(listed) == 0 && len(repeated) > 0 {
		return c.sendRestartNote(ctx, now, repeated)
	}
	return c.sendRestored(ctx, now, listed, lower)
}

// restoredNews sorts the restored incidents still failing: listed are
// the ones to list now, repeated the ones a recent listing named (only
// gathered when final), and lower the lower-priority ones the
// digest lists.
func (c *Collector) restoredNews(now time.Time, final bool) (
	listed, repeated, lower []incident.Decision,
) {
	for _, p := range c.env.Incidents.RestoredFailing() {
		d := incident.Decision{Action: incident.Announce, Incident: p,
			Reason: "restored"}
		switch {
		case c.env.Scope != nil && !c.env.Scope(p):
		case c.restoredListed[p.ID]:
		case p.Tier < incident.Notify || p.DigestBound():
			// The next digest lists it (see ongoing_compact.go). One
			// whose findings only wait for the digest (an idle
			// webhook) was told once and falls there after the
			// grace; a restart does not announce it again.
			lower = append(lower, d)
			c.markRestoredListed(p.ID)
		case c.recentlyListed(p.ID, now):
			if final {
				repeated = append(repeated, d)
				c.markRestoredListed(p.ID)
			}
		case !final && !currentNews(p):
			// Its tier may still fall when its findings settle: it
			// waits for the end of the grace.
		default:
			listed = append(listed, d)
			c.markRestoredListed(p.ID)
		}
	}
	return listed, repeated, lower
}

// listLateRestored names the restored incidents whose findings came back
// after the restore grace, as an ordinary restored listing: a pod that
// the morning wake-up recreates is held back past the grace, and its
// incident was announced before the restart, so nothing else would say
// it still fails. It watches for as long as a wake-up may hold findings.
func (c *Collector) listLateRestored(
	ctx context.Context, now time.Time,
) bool {
	s := &c.Startup
	if s.lateUntil.IsZero() {
		return false
	}
	if !now.Before(s.lateUntil) {
		s.lateUntil = time.Time{}
		c.restoredListed = nil
		return false
	}
	var listed []incident.Decision
	for _, p := range c.env.Incidents.RestoredFailing() {
		if c.restoredListed[p.ID] || !currentNews(p) ||
			c.recentlyListed(p.ID, now) ||
			(c.env.Scope != nil && !c.env.Scope(p)) {
			continue
		}
		listed = append(listed, incident.Decision{
			Action: incident.Announce, Incident: p, Reason: "restored"})
		c.markRestoredListed(p.ID)
	}
	return c.sendRestored(ctx, now, listed, nil)
}

// sendRestored sends the restored-incidents message for listed, if any.
func (c *Collector) sendRestored(
	ctx context.Context, now time.Time,
	listed, lower []incident.Decision,
) bool {
	if len(listed) == 0 {
		return false
	}
	msg := c.env.Messages.RestoredSummaryWith(listed, len(lower), now)
	// The audit entry has no incident of its own: name the restored ones,
	// the lower-priority ones too.
	msg.Listed = restoredListing(listed, lower...)
	klog.InfoS("pipeline: restored incidents still failing",
		append(restoredTotals(listed), "lowerPriority", len(lower))...)
	// The entry's tier is the loudest listed one, so a listing that
	// names a page does not read as silent.
	c.env.Sink(ctx, incident.Decision{Reason: "restored incidents",
		Incident: incident.Incident{Tier: loudest(listed)}}, msg)
	c.rememberListed(listed, now)
	return true
}

// earlyListingDue reports that the sources synced a settle ago and the
// early listing was not sent yet.
func (c *Collector) earlyListingDue(now time.Time) bool {
	s := &c.Startup
	return !s.earlyDone && !s.SyncedAt.IsZero() &&
		!now.Before(s.SyncedAt.Add(incident.DefaultSettle))
}

// markRestoredListed remembers that the early listing named incident id,
// so the one at the end of the grace does not name it again.
func (c *Collector) markRestoredListed(id string) {
	if c.restoredListed == nil {
		c.restoredListed = map[string]bool{}
	}
	c.restoredListed[id] = true
}

// NextWarm is when the restored incidents are next listed, or zero: the
// early listing while it is still due, else the end of the grace.
func (c *Collector) NextWarm() time.Time {
	s := &c.Startup
	if s.WarmAt.IsZero() {
		return time.Time{}
	}
	if !s.earlyDone && !s.SyncedAt.IsZero() {
		early := s.SyncedAt.Add(incident.DefaultSettle)
		if early.Before(s.WarmAt) {
			return early
		}
	}
	return s.WarmAt
}

// currentNews reports a restored incident at the notify tier or higher
// whose findings are still notify or page news now, so its tier will not
// fall when the grace is over.
func currentNews(p incident.Incident) bool {
	return p.Tier >= incident.Notify && p.LiveTier() >= incident.Notify &&
		!p.DigestBound()
}
