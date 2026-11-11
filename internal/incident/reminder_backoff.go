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

// resetBackoff starts the reminder gaps over when a message other than
// a reminder is sent: a rise, a new cause or new objects is news.
func (p *Incident) resetBackoff(why Reason) {
	if why != ReasonReminder {
		p.Reminders = 0
	}
}
