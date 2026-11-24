package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/format"
)

// chronicBackoffSteps are the gaps between the "still open" updates of
// an incident that has no message of its own, after its announcement:
// a day, then 3 days, then a week, and the last gap again and again.
// News resets it (see resetBackoff), so a problem that changes is said
// again on the normal cadence.
var chronicBackoffSteps = []time.Duration{
	ChronicRemindEvery, 3 * format.Day, format.Week,
}

// chronicBackoff is the gap before reminder number count+1.
func chronicBackoff(count int) time.Duration {
	if count >= len(chronicBackoffSteps) {
		count = len(chronicBackoffSteps) - 1
	}
	return chronicBackoffSteps[max(count, 0)]
}

// noteReminded records that a "still open" update was sent at now.
func (p *Incident) noteReminded(now time.Time) {
	p.Reminded = now
	p.Reminders++
}

// resetBackoff starts the reminder gaps over when a message is news: a
// rise, a new cause or new objects. A reminder is not, nor is an
// introduction (it says in full what a listing named) or a lowered
// tier (chat is not told of it): they keep the back-off.
func (p *Incident) resetBackoff(why Reason) {
	if isNews(why) {
		p.Reminders = 0
	}
}

// isNews reports a decision that tells something new about the problem.
func isNews(why Reason) bool {
	switch why {
	case ReasonReminder, ReasonIntroduced, ReasonTierLowered:
		return false
	}
	return true
}

// noteNews starts the reminder gaps over on news (resetBackoff) and
// dates it (Marks.NewsAt) at the time of the manager's latest run: an
// update or a reopening that tells something new. An announcement is
// the first telling, not a change.
func (m *Manager) noteNews(p *Incident, action Action, why Reason) {
	p.resetBackoff(why)
	if action != Announce && isNews(why) && !m.latest.IsZero() {
		p.marks.NewsAt = m.latest
	}
}

// unchangedSince is when the problem of p last changed: its latest news
// (Marks.NewsAt), or, when there was none since, when its failure began.
// A deletion stuck for a year that kwatch first saw today has been
// unchanged for a year.
func unchangedSince(p *Incident) time.Time {
	since := p.Opened
	for _, s := range p.Members {
		if !s.Advisory && !s.Since.IsZero() && s.Since.Before(since) {
			since = s.Since
		}
	}
	if p.marks.NewsAt.After(since) {
		return p.marks.NewsAt
	}
	return since
}

// unchangedGap is the least gap between two reminders of p: a problem
// unchanged for UnchangedFor is reminded at most once per UnchangedFor.
func unchangedGap(
	p *Incident, now time.Time, every time.Duration,
) time.Duration {
	since := unchangedSince(p)
	if !since.IsZero() && now.Sub(since) >= UnchangedFor {
		return max(every, UnchangedFor)
	}
	return every
}
