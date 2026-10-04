package pipeline

import (
	"context"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// Listing is one message that named several incidents instead of one
// message each: the startup summary, or a roll-up of problems found at
// the same time. It remembers what the message listed, so the first own
// message of a listed incident introduces it and the listing is closed
// once everything it named has resolved.
type Listing struct {
	// Key is the conversation key of the message sent.
	Key string `json:",omitempty"`
	// Incidents lists the incidents the message still waits on.
	Incidents []string `json:",omitempty"`
	// Followed lists the listed incidents that have since sent a
	// message of their own, and Resolved those whose own resolve was
	// sent. A listing without them reads as "nothing sent yet", which
	// costs at most one extra message.
	Followed []string `json:",omitempty"`
	Resolved []string `json:",omitempty"`
}

// StartupState is the persisted startup summary marker, with the
// roll-ups still open. It makes a cold start explicit instead of
// inferring it from an empty incident store. The embedded Listing is the
// startup summary itself; its fields are stored flat, as earlier
// versions wrote them.
type StartupState struct {
	// Complete is true once a startup summary was handed to delivery, or
	// the startup window closed with nothing to report.
	Complete bool
	Listing
	// Rollups are the roll-ups whose incidents have not all resolved.
	Rollups []Listing `json:",omitempty"`
}

// listings returns the startup summary and every open roll-up.
func (s *StartupState) listings() []*Listing {
	out := []*Listing{&s.Listing}
	for i := range s.Rollups {
		out = append(out, &s.Rollups[i])
	}
	return out
}

// clone copies the state deeply, for a writer on another goroutine.
func (s StartupState) clone() StartupState {
	s.Listing = s.Listing.clone()
	s.Rollups = slices.Clone(s.Rollups)
	for i := range s.Rollups {
		s.Rollups[i] = s.Rollups[i].clone()
	}
	return s
}

func (l Listing) clone() Listing {
	l.Incidents = slices.Clone(l.Incidents)
	l.Followed = slices.Clone(l.Followed)
	l.Resolved = slices.Clone(l.Resolved)
	return l
}

// follow records an own message of an incident the listing named. It
// reports whether d is about such an incident; the decision it returns
// is the one to write the message for: the listing only named the
// incident, so its first own message is a full announcement, not a bare
// update.
func (l *Listing) follow(d incident.Decision) (incident.Decision, bool) {
	id := d.Incident.ID
	if !slices.Contains(l.Incidents, id) {
		return d, false
	}
	first := !slices.Contains(l.Followed, id)
	if first {
		l.Followed = append(l.Followed, id)
	}
	if d.Action == incident.Resolve {
		l.Resolved = append(l.Resolved, id)
	}
	if first && d.Action == incident.Update {
		d.Action = incident.Announce
	}
	return d, true
}

// closes reports whether the listing is over: every incident it named
// has resolved, and whether people must be told. When every listed
// incident already said it resolved, one more "all resolved" would be a
// repeated recovery.
func (l *Listing) closes(incidents *incident.Manager) (over, tell bool) {
	if len(l.Incidents) == 0 || !incidents.Closed(l.Incidents) {
		return false, false
	}
	return true, !containsAll(l.Resolved, l.Incidents)
}

// startupWindow covers the initial list plus one settle period, so the
// incidents that already existed are all in the summary.
const startupWindow = 2 * time.Minute

// startupSummary is the announcer's cold-start state. While the startup
// window is open, announcements of incidents that already existed are
// collected instead of sent. Afterwards it remembers the summary that
// was sent, so the summary can be resolved once everything it listed
// has resolved.
type startupSummary struct {
	// coldStart is true until a startup summary was completed for this
	// installation.
	coldStart bool
	// until is when the startup window closes; zero while none is open.
	until time.Time
	// collected are the announcements the summary will list.
	collected []incident.Decision
	// summary is the last startup summary sent, with the open roll-ups.
	summary StartupState
	// checkSummary asks the next tick whether every incident a listing
	// named has resolved.
	checkSummary bool
}

// openWindow starts collecting the startup summary at now, on a cold
// start only.
func (s *startupSummary) openWindow(now time.Time) {
	if s.coldStart {
		s.until = now.Add(startupWindow)
	}
}

// collectStartup holds cold-start announcements until the startup window
// ends, then sends them as one summary. Only incidents that were already
// failing when the sources synced are held: a problem that starts later
// is news and is announced at once. A page-tier announcement is held for
// the chat summary too, and sent on its own to the paging tools and issue
// trackers, which never receive summaries, so an active page is not
// delayed by the window. Updates and resolves of a held incident fold
// into the held entry. It reports whether the summary window finished in
// this call.
func (a *announcer) collectStartup(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	if a.startup.until.IsZero() {
		return decisions, false
	}
	var rest []incident.Decision
	for _, d := range decisions {
		if !a.holdStartup(ctx, now, d) {
			rest = append(rest, d)
		}
	}
	if now.Before(a.startup.until) {
		return rest, false
	}
	a.finishStartup(ctx, now)
	return rest, true
}

// holdStartup reports whether d was absorbed by the startup summary. A
// held decision still reaches the sink for the audit log: as a paging-only
// message when it is a page-tier announcement, otherwise marked as carried
// by the summary.
func (a *announcer) holdStartup(
	ctx context.Context, now time.Time, d incident.Decision,
) bool {
	s := &a.startup
	id := d.Incident.ID
	held := s.indexOf(id)
	switch {
	case d.Action == incident.Announce && s.predatesSync(d.Incident):
		if held >= 0 {
			s.collected[held] = d
		} else {
			s.collected = append(s.collected, d)
		}
		a.incidents.HoldAnnouncement(id)
		if held < 0 && d.Incident.Tier == incident.Page {
			msg := a.write(d, now)
			msg.PagingOnly = true
			a.sink(ctx, d, msg)
			return true
		}
		a.recordCarried(ctx, now, d, "startup summary")
		return true
	case held >= 0 && d.Action == incident.Update:
		s.collected[held].Incident = d.Incident
		a.recordCarried(ctx, now, d, "startup summary")
		return true
	case held >= 0 && d.Action == incident.Resolve:
		// Resolved before anyone was told: leave it out of the summary.
		s.collected = append(s.collected[:held], s.collected[held+1:]...)
		a.incidents.ReleaseAnnouncement(id)
		a.recordCarried(ctx, now, d, "startup summary")
		return true
	}
	return false
}

// predatesSync reports whether p was already failing when the sources
// synced: its earliest finding, or its opening, is not after that
// moment. An incident without any known time is treated as old.
func (s *startupSummary) predatesSync(p incident.Incident) bool {
	synced := s.until.Add(-startupWindow)
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
func (s *startupSummary) indexOf(id string) int {
	for i, d := range s.collected {
		if d.Incident.ID == id {
			return i
		}
	}
	return -1
}

// finishStartup sends the summary, then releases the held announcements
// so they are persisted as announced only after delivery has them.
func (a *announcer) finishStartup(ctx context.Context, now time.Time) {
	s := &a.startup
	state := StartupState{Complete: true, Rollups: s.summary.Rollups}
	if len(s.collected) > 0 {
		msg := a.messages.StartupSummary(s.collected, now)
		a.sink(ctx, incident.Decision{Reason: "startup summary"}, msg)
		state.Listing = a.listed(msg.Key, s.collected)
	}
	s.summary = state
	a.storage.saveStartup(state)
	s.collected, s.until = nil, time.Time{}
}

// listed builds the listing of the message with key that named the
// announcements, and releases their held announcements: delivery has the
// message now.
func (a *announcer) listed(
	key string, announcements []incident.Decision,
) Listing {
	l := Listing{Key: key}
	for _, d := range announcements {
		l.Incidents = append(l.Incidents, d.Incident.ID)
		a.incidents.ReleaseAnnouncement(d.Incident.ID)
	}
	return l
}

// closeSummary resolves the startup summary and every roll-up whose
// listed incidents have all resolved. It reports whether it sent a
// resolve.
func (a *announcer) closeSummary(ctx context.Context) bool {
	s := &a.startup
	if !s.checkSummary {
		return false
	}
	s.checkSummary = false
	sent, changed := false, false
	if over, tell := s.summary.Listing.closes(a.incidents); over {
		if tell {
			a.sink(ctx, incident.Decision{
				Reason: "startup summary resolved"},
				a.messages.StartupResolved(s.summary.Key,
					len(s.summary.Incidents)))
		}
		s.summary.Listing = Listing{}
		sent, changed = sent || tell, true
	}
	open := s.summary.Rollups[:0]
	for _, r := range s.summary.Rollups {
		over, tell := r.closes(a.incidents)
		if !over {
			open = append(open, r)
			continue
		}
		if tell {
			a.sink(ctx, incident.Decision{Reason: "roll-up resolved"},
				a.messages.RollupResolved(r.Key, len(r.Incidents)))
		}
		sent, changed = sent || tell, true
	}
	s.summary.Rollups = open
	if changed {
		a.storage.saveStartup(s.summary)
	}
	return sent
}

// followSummary records the own messages of incidents the startup
// summary or a roll-up listed. The listing only named them, so the first
// message of their own is written as a full announcement, not as a bare
// update. It returns the decision to write the message for.
func (a *announcer) followSummary(d incident.Decision) incident.Decision {
	for _, l := range a.startup.summary.listings() {
		if out, listed := l.follow(d); listed {
			a.storage.saveStartup(a.startup.summary)
			return out
		}
	}
	return d
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
