package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// deliveredOK is the result of a message a channel accepted.
const deliveredOK = "ok"

// timingOf is when p's failure started, when kwatch noticed it and when
// it first and last told a channel, from its findings, its open time
// and the delivery trail (oldest first); nil when none is known. The
// first send counts from the issue's first opening, not its latest
// reopen: a reopen resets Opened, and "told at" must not move with it.
func timingOf(p incident.Incident, trail []DeliveryAttempt) *Timing {
	t := Timing{NoticedAt: p.Opened, StartedAt: startedAt(p)}
	from := firstOpened(p)
	for _, a := range trail {
		if a.Result != deliveredOK || a.At.Before(from) {
			continue
		}
		if t.ToldAt.IsZero() || a.At.Before(t.ToldAt) {
			t.ToldAt, t.ToldVia = a.At, channelName(a.Channel)
		}
		if !a.At.Before(t.LastToldAt) {
			t.LastToldAt = a.At
			t.LastToldVia = channelName(a.Channel)
		}
	}
	if t == (Timing{}) {
		return nil
	}
	return &t
}

// firstOpened is when the issue first opened: the first of the
// openings kwatch kept, else its open time.
func firstOpened(p incident.Incident) time.Time {
	if o := openings(p); len(o) > 0 && o[0].Before(p.Opened) {
		return o[0]
	}
	return p.Opened
}

// failedAgain is how many times the issue came back after it first
// opened: its openings kept (reopens, recurrences and silent returns
// alike) less the first. The list, the overview's trends and the issue
// page all show this one count ("failed again N times").
func failedAgain(p incident.Incident) int {
	return max(len(openings(p))-1, 0)
}

// startedAt is the earliest start of p's failure findings, never after
// p opened; zero when no finding says.
func startedAt(p incident.Incident) time.Time {
	var first time.Time
	for _, f := range p.Members {
		if f.Advisory || f.Since.IsZero() {
			continue
		}
		if first.IsZero() || f.Since.Before(first) {
			first = f.Since
		}
	}
	if !p.Opened.IsZero() && first.After(p.Opened) {
		return p.Opened
	}
	return first
}
