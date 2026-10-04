package pipeline

import (
	"context"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// StartupState is the persisted startup summary marker. It makes a cold
// start explicit instead of inferring it from an empty incident store, and
// lets the summary be closed once everything it listed has resolved.
type StartupState struct {
	// Complete is true once a startup summary was handed to delivery, or
	// the startup window closed with nothing to report.
	Complete bool
	// Key is the conversation key of the last summary sent.
	Key string `json:",omitempty"`
	// Incidents lists the incidents that summary still waits on.
	Incidents []string `json:",omitempty"`
	// Followed lists the listed incidents that have since sent a
	// message of their own, and Resolved those whose own resolve was
	// sent. A marker without them reads as "nothing sent yet", which
	// costs at most one extra message.
	Followed []string `json:",omitempty"`
	Resolved []string `json:",omitempty"`
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
	// summary is the last startup summary sent.
	summary StartupState
	// checkSummary asks the next tick whether every incident the
	// summary listed has resolved.
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

// holdStartup reports whether d was absorbed by the startup summary.
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
		}
		return true
	case held >= 0 && d.Action == incident.Update:
		s.collected[held].Incident = d.Incident
		return true
	case held >= 0 && d.Action == incident.Resolve:
		// Resolved before anyone was told: leave it out of the summary.
		s.collected = append(s.collected[:held], s.collected[held+1:]...)
		a.incidents.ReleaseAnnouncement(id)
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
	state := StartupState{Complete: true}
	if len(s.collected) > 0 {
		msg := a.messages.StartupSummary(s.collected, now)
		a.sink(ctx, incident.Decision{Reason: "startup summary"}, msg)
		state.Key = msg.Key
		for _, d := range s.collected {
			state.Incidents = append(state.Incidents, d.Incident.ID)
			a.incidents.ReleaseAnnouncement(d.Incident.ID)
		}
	}
	s.summary = state
	a.storage.saveStartup(state)
	s.collected, s.until = nil, time.Time{}
}

// closeSummary resolves the startup summary once every incident it listed
// has resolved. It reports whether it sent the resolve.
func (a *announcer) closeSummary(ctx context.Context) bool {
	s := &a.startup
	if !s.checkSummary {
		return false
	}
	s.checkSummary = false
	ids := s.summary.Incidents
	if len(ids) == 0 || !a.incidents.Closed(ids) {
		return false
	}
	// When every listed incident already said it resolved, one more
	// "all resolved" would be a repeated recovery.
	sent := !containsAll(s.summary.Resolved, ids)
	if sent {
		a.sink(ctx, incident.Decision{
			Reason: "startup summary resolved"},
			a.messages.StartupResolved(s.summary.Key, len(ids)))
	}
	s.summary.Incidents, s.summary.Followed = nil, nil
	s.summary.Resolved = nil
	a.storage.saveStartup(s.summary)
	return sent
}

// followSummary records the own messages of incidents the startup
// summary listed. The summary only named them, so the first message of
// their own is written as a full announcement, not as a bare update.
// It returns the decision to write the message for.
func (a *announcer) followSummary(d incident.Decision) incident.Decision {
	summary := &a.startup.summary
	id := d.Incident.ID
	if !slices.Contains(summary.Incidents, id) {
		return d
	}
	first := !slices.Contains(summary.Followed, id)
	if first {
		summary.Followed = append(summary.Followed, id)
	}
	if d.Action == incident.Resolve {
		summary.Resolved = append(summary.Resolved, id)
	}
	a.storage.saveStartup(*summary)
	if first && d.Action == incident.Update {
		d.Action = incident.Announce
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
