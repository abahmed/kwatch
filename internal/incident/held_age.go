package incident

import (
	"regexp"
	"time"

	"github.com/abahmed/kwatch/internal/format"
)

// announceSettled marks a settled incident told at now. One a scheduled
// scale-down held (planned_hold.go) is told hours after its findings
// were written, so the ages their summaries state are brought up to
// now first: a node held overnight has not reported for 13 hours, not
// for the two minutes it had when it was first seen.
func (m *Manager) announceSettled(p *Incident, now time.Time) {
	if _, held := m.plannedHoldEnd(p); held {
		for key, f := range p.Members {
			f.Summary = refreshAge(f.Summary, f.Since, now)
			p.Members[key] = f
		}
	}
	p.State, p.Announced = Open, now
}

// statedAge matches the age a detector writes into a summary with
// format.Duration: "for 2m", "for 1h5m", "for 45s".
var statedAge = regexp.MustCompile(
	`\bfor ((?:\d+h)?(?:\d+m)?(?:\d+s)?)\b`)

// refreshAge rewrites the first age summary states ("for 2m") to the
// time since since, when that age is the one the summary measured: it
// is no longer than the time since since. A summary without an age,
// one whose duration is not an age, or a finding without an onset is
// returned as it is.
func refreshAge(summary string, since, now time.Time) string {
	if since.IsZero() || !now.After(since) {
		return summary
	}
	age := now.Sub(since)
	for _, loc := range statedAge.FindAllStringSubmatchIndex(summary, -1) {
		stated, err := time.ParseDuration(summary[loc[2]:loc[3]])
		if err != nil || stated > age {
			continue
		}
		return summary[:loc[2]] + format.Duration(age) + summary[loc[3]:]
	}
	return summary
}
