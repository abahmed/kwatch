package announce

import (
	"context"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ListRestored sends the one message of a restart, once the findings of
// the incidents restored from the previous session are back (WarmAt):
// the downtime gap and what changed meanwhile (the gap report, when one
// was held, see HoldGap), the restored incidents that are news, and how
// many ongoing ones are unchanged. A restored incident is news when
// nobody was told about it yet or it got worse since it was told
// (restoredIsNews); one told and unchanged is only counted. Problems
// found after the restart that were failing since before it (held by
// holdWarm) are news too. With no news and no gap nothing is sent: a
// quiet restart stays quiet, and only the audit log records it. It
// reports whether it sent a message.
func (c *Collector) ListRestored(ctx context.Context, now time.Time) bool {
	s := &c.Startup
	if s.WarmAt.IsZero() {
		return c.listLateRestored(ctx, now)
	}
	if now.Before(s.WarmAt) {
		return false
	}
	news, ongoing := c.restoredNews()
	news = c.takeWarm(news)
	// Findings held by a wake-up or a fresh pod's grace come back after
	// the grace: keep watching for them (listLateRestored).
	s.lateUntil = s.WarmAt.Add(kube.WakeMax)
	s.WarmAt = time.Time{}
	c.noteRestoredIntros()
	return c.sendRestart(ctx, now, news, ongoing)
}

// restoredNews sorts the restored incidents still failing that people
// want to hear about: news are the ones to name, ongoing the ones
// already told and unchanged, which are only counted. Only the news is
// remembered as named by the restored summary (ListedBy): a count names
// nothing, so it owes no introduction (restored_intro.go).
func (c *Collector) restoredNews() (news, ongoing []incident.Decision) {
	for _, p := range c.env.Incidents.RestoredFailing() {
		if c.restoredSeen(p.ID) ||
			(c.env.Scope != nil && !c.env.Scope(p)) {
			continue
		}
		d := incident.Decision{Action: incident.Announce, Incident: p,
			Reason: "restored"}
		if !c.restoredIsNews(p) {
			c.markRestoredCounted(p.ID)
			ongoing = append(ongoing, d)
			continue
		}
		c.markRestoredListed(p.ID)
		// A worse one is named at the tier its findings have now.
		d.Incident.Tier = max(p.Tier, p.LiveTier())
		news = append(news, d)
	}
	return news, ongoing
}

// restoredIsNews reports a restored incident at the notify tier or
// higher, whose findings are still news (currentNews), that nobody was
// told about yet or whose findings are now louder than the tier it was
// told at. A lower-priority one waits for the digest (see
// ongoing_compact.go).
func (c *Collector) restoredIsNews(p incident.Incident) bool {
	if !currentNews(p) {
		return false
	}
	return !c.toldBefore(p) || p.LiveTier() > p.Tier
}

// toldBefore reports a restored incident some message already told:
// its own, a page, a listing that named it, or one of its own after a
// listing (StartupState.Spoke).
func (c *Collector) toldBefore(p incident.Incident) bool {
	if p.Told() {
		return true
	}
	s := c.Startup.Summary
	if _, ok := s.Spoke[p.ID]; ok {
		return true
	}
	if s.Restored == nil {
		return false
	}
	_, named := s.Restored.Told[p.ID]
	return named || slices.Contains(s.Restored.IDs, p.ID)
}

// listLateRestored names the restored incidents whose findings came back
// after the restore grace and that are news: a pod that the morning
// wake-up recreates is held back past the grace. One told and unchanged
// is not named again. It watches for as long as a wake-up may hold
// findings.
func (c *Collector) listLateRestored(
	ctx context.Context, now time.Time,
) bool {
	s := &c.Startup
	if s.lateUntil.IsZero() {
		return false
	}
	if !now.Before(s.lateUntil) {
		s.lateUntil = time.Time{}
		c.restoredListed, c.restoredCounted = nil, nil
		return false
	}
	var news []incident.Decision
	for _, p := range c.env.Incidents.RestoredFailing() {
		if c.restoredSeen(p.ID) || !currentNews(p) ||
			(c.env.Scope != nil && !c.env.Scope(p)) {
			continue
		}
		if !c.restoredIsNews(p) {
			c.markRestoredCounted(p.ID)
			continue
		}
		c.markRestoredListed(p.ID)
		d := incident.Decision{Action: incident.Announce,
			Incident: p, Reason: "restored"}
		d.Incident.Tier = max(p.Tier, p.LiveTier())
		news = append(news, d)
	}
	if len(news) == 0 {
		return false
	}
	return c.sendRestart(ctx, now, news, nil)
}

// markRestoredListed remembers that the restored summary named incident
// id, so it is not named again in this session.
func (c *Collector) markRestoredListed(id string) {
	if c.restoredListed == nil {
		c.restoredListed = map[string]bool{}
	}
	c.restoredListed[id] = true
}

// markRestoredCounted remembers that the restored summary only counted
// incident id, unchanged: it is not looked at again in this session,
// and it is not named (ListedBy).
func (c *Collector) markRestoredCounted(id string) {
	if c.restoredCounted == nil {
		c.restoredCounted = map[string]bool{}
	}
	c.restoredCounted[id] = true
}

// restoredSeen reports an incident the restored summary named or
// counted in this session.
func (c *Collector) restoredSeen(id string) bool {
	return c.restoredListed[id] || c.restoredCounted[id]
}

// NextWarm is when the restored incidents are listed, or zero.
func (c *Collector) NextWarm() time.Time {
	return c.Startup.WarmAt
}

// currentNews reports a restored incident at the notify tier or higher
// whose findings are still notify or page news now, so its tier will not
// fall when the grace is over.
func currentNews(p incident.Incident) bool {
	return p.Tier >= incident.Notify && p.LiveTier() >= incident.Notify &&
		!p.DigestBound()
}
