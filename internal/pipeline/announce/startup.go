package announce

import (
	"context"
	"slices"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
)

// StartupWindow covers the initial list plus one settle period, so the
// incidents that already existed are all in the summary.
const StartupWindow = 2 * time.Minute

// Startup is the collector's cold-start state. While the startup
// window is open, announcements of incidents that already existed are
// collected instead of sent. Afterwards it remembers the summary that
// was sent, so the summary can be resolved once everything it listed
// has resolved.
type Startup struct {
	// ColdStart is true until a startup summary was completed for this
	// installation.
	ColdStart bool
	// Until is when the startup window closes; zero while none is open.
	Until time.Time
	// Collected are the announcements the summary will list.
	Collected []incident.Decision
	// Summary is the last startup summary sent, with the open roll-ups.
	Summary StartupState
	// WarmAt is when the incidents restored from the previous session
	// are listed, once their findings had time to come back; zero when
	// there is nothing to list. A cold start lists nothing here: its
	// summary names every existing incident.
	WarmAt time.Time
	// CheckSummary asks the next tick whether every incident a listing
	// named has resolved.
	CheckSummary bool
}

// OpenWindow starts collecting the startup summary at now, on a cold
// start only.
func (s *Startup) OpenWindow(now time.Time) {
	if s.ColdStart {
		s.Until = now.Add(StartupWindow)
	}
}

// CollectStartup holds cold-start announcements until the startup window
// ends, then sends them as one summary. Only incidents that were already
// failing when the sources synced are held: a problem that starts later
// is news and is announced at once. A page-tier announcement is held for
// the chat summary too, and sent on its own to the paging tools and issue
// trackers, which never receive summaries, so an active page is not
// delayed by the window. Updates and resolves of a held incident fold
// into the held entry. It reports whether the summary window finished in
// this call.
func (c *Collector) CollectStartup(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	if c.Startup.Until.IsZero() {
		return decisions, false
	}
	var rest []incident.Decision
	for _, d := range decisions {
		if !c.holdStartup(ctx, now, d) {
			rest = append(rest, d)
		}
	}
	if now.Before(c.Startup.Until) {
		return rest, false
	}
	c.finishStartup(ctx, now)
	return rest, true
}

// holdStartup reports whether d was absorbed by the startup summary. A
// held decision still reaches the sink for the audit log: as a paging-only
// message when it is a page-tier announcement, otherwise marked as carried
// by the summary.
func (c *Collector) holdStartup(
	ctx context.Context, now time.Time, d incident.Decision,
) bool {
	s := &c.Startup
	id := d.Incident.ID
	held := s.indexOf(id)
	switch {
	case d.Action == incident.Announce && s.predatesSync(d.Incident):
		if held >= 0 {
			s.Collected[held] = d
		} else {
			s.Collected = append(s.Collected, d)
		}
		c.env.Incidents.HoldAnnouncement(id)
		c.env.Incidents.RecordRolledUp(id)
		if held < 0 && d.Incident.Tier == incident.Page {
			c.pageOnly(ctx, now, d)
			return true
		}
		c.recordCarried(ctx, now, d, "startup summary")
		return true
	case held >= 0 && d.Action == incident.Update:
		s.Collected[held].Incident = d.Incident
		if c.risesToPage(d) {
			// The summary goes to chat only: the page is not delayed.
			c.pageOnly(ctx, now, d)
			return true
		}
		c.recordCarried(ctx, now, d, "startup summary")
		return true
	case held >= 0 && d.Action == incident.Resolve:
		// Resolved before anyone was told: leave it out of the summary.
		s.Collected = append(s.Collected[:held], s.Collected[held+1:]...)
		c.closeUnannounced(ctx, now, d, "startup summary")
		return true
	}
	return false
}

// predatesSync reports whether p was already failing when the sources
// synced: its earliest finding, or its opening, is not after that
// moment. An incident without any known time is treated as old.
func (s *Startup) predatesSync(p incident.Incident) bool {
	synced := s.Until.Add(-StartupWindow)
	first := p.Opened
	for _, member := range p.Members {
		if !member.Since.IsZero() &&
			(first.IsZero() || member.Since.Before(first)) {
			first = member.Since
		}
	}
	return first.IsZero() || !first.After(synced)
}

// indexOf returns the position of incident id among the collected
// announcements, or -1.
func (s *Startup) indexOf(id string) int {
	for i, d := range s.Collected {
		if d.Incident.ID == id {
			return i
		}
	}
	return -1
}

// finishStartup sends the summary, then releases the held announcements
// so they are persisted as announced only after delivery has them.
func (c *Collector) finishStartup(ctx context.Context, now time.Time) {
	s := &c.Startup
	state := StartupState{Complete: true, Rollups: s.Summary.Rollups,
		Digest: s.Summary.Digest}
	if len(s.Collected) > 0 {
		msg := c.env.Messages.StartupSummary(s.Collected, now)
		c.env.Sink(ctx, incident.Decision{Reason: "startup summary"}, msg)
		state.Listing = c.listed(msg.Key, s.Collected)
	}
	s.Summary = state
	c.env.SaveStartup(state)
	s.Collected, s.Until = nil, time.Time{}
}

// listed builds the listing of the message with key that named the
// announcements, and releases their held announcements: delivery has the
// message now.
func (c *Collector) listed(
	key string, announcements []incident.Decision,
) Listing {
	l := Listing{Key: key}
	for _, d := range announcements {
		l.Incidents = append(l.Incidents, d.Incident.ID)
		c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
	}
	return l
}

// CloseListings resolves the startup summary and every roll-up whose
// listed incidents have all resolved. It reports whether it sent a
// resolve.
func (c *Collector) CloseListings(ctx context.Context) bool {
	s := &c.Startup
	if !s.CheckSummary {
		return false
	}
	s.CheckSummary = false
	sent, changed := false, false
	if over, tell := s.Summary.Listing.closes(c.env.Incidents); over {
		if tell {
			c.env.Sink(ctx, incident.Decision{
				Reason: "startup summary resolved"},
				c.env.Messages.StartupResolved(s.Summary.Key,
					len(s.Summary.Incidents)))
		}
		s.Summary.Listing = Listing{}
		sent, changed = sent || tell, true
	}
	open := s.Summary.Rollups[:0]
	for _, r := range s.Summary.Rollups {
		over, tell := r.closes(c.env.Incidents)
		if !over {
			open = append(open, r)
			continue
		}
		if tell {
			c.env.Sink(ctx, incident.Decision{Reason: "roll-up resolved"},
				c.env.Messages.RollupResolved(r.Key, len(r.Incidents)))
		}
		sent, changed = sent || tell, true
	}
	s.Summary.Rollups = open
	if changed {
		c.env.SaveStartup(s.Summary)
	}
	return sent
}

// Follow records the own messages of incidents the startup
// summary or a roll-up listed. The listing only named them, so the first
// message of their own is written as a full announcement, not as a bare
// update. It returns the decision to write the message for.
func (c *Collector) Follow(d incident.Decision) incident.Decision {
	for _, l := range c.Startup.Summary.listings() {
		if out, listed := l.follow(d); listed {
			c.env.SaveStartup(c.Startup.Summary)
			return out
		}
	}
	return d
}

// NoteResolveDelivered records that the resolve of incident id reached
// chat, for every listing that named it. Only then does the listing count
// the incident as having said it resolved: a resolve that went to the
// pagers alone, or that nobody received, leaves the listing to say so
// when everything it named is over.
func (c *Collector) NoteResolveDelivered(id string) {
	changed := false
	for _, l := range c.Startup.Summary.listings() {
		changed = l.noteResolved(id) || changed
	}
	if changed {
		c.env.SaveStartup(c.Startup.Summary)
	}
}

// containsAll reports whether every one of want is in have.
func containsAll(have, want []string) bool {
	for _, id := range want {
		if !slices.Contains(have, id) {
			return false
		}
	}
	return true
}

// ListRestored sends one message naming the incidents restored from the
// previous session that still fail, once the restore grace is over: a
// restart announces none of them, so without it nobody would hear that
// they are still open. It reports whether it sent the message.
func (c *Collector) ListRestored(ctx context.Context, now time.Time) bool {
	s := &c.Startup
	if s.WarmAt.IsZero() || now.Before(s.WarmAt) {
		return false
	}
	s.WarmAt = time.Time{}
	var listed []incident.Decision
	for _, p := range c.env.Incidents.RestoredFailing() {
		if c.env.Scope == nil || c.env.Scope(p) {
			listed = append(listed, incident.Decision{
				Action: incident.Announce, Incident: p,
				Reason: "restored"})
		}
	}
	if len(listed) == 0 {
		return false
	}
	msg := c.env.Messages.RestoredSummary(listed, now)
	// The audit entry has no incident of its own: name the restored ones.
	msg.Listed = restoredListing(listed)
	klog.InfoS("pipeline: restored incidents still failing",
		restoredTotals(listed)...)
	c.env.Sink(ctx, incident.Decision{Reason: "restored incidents"}, msg)
	return true
}

// NextStartup is when the startup window closes and the summary is sent,
// or zero when none is open.
func (c *Collector) NextStartup() time.Time {
	return c.Startup.Until
}

// NextWarm is when the restored incidents are listed, or zero.
func (c *Collector) NextWarm() time.Time {
	return c.Startup.WarmAt
}

// Restore sets the cold-start state from the previous run. marker is the
// startup state it saved and found says one was saved; restored is how
// many incidents came back and grace when their findings are due. A
// store without a marker falls back to the record count: no records
// means a cold start.
func (s *Startup) Restore(
	marker StartupState, found bool, restored int, grace time.Time,
) {
	s.ColdStart = !marker.Complete
	if !found {
		s.ColdStart = restored == 0
	}
	// Roll-ups and the digest of a run that stopped inside its startup
	// window are still owed their resolve and their lines.
	s.Summary = marker
	s.CheckSummary = len(marker.Incidents) > 0 || len(marker.Rollups) > 0
	if s.ColdStart {
		return
	}
	if restored > 0 {
		s.WarmAt = grace
	}
}
