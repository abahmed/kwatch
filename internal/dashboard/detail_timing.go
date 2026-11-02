package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// deliveredOK is the result of a message a channel accepted.
const deliveredOK = "ok"

// timingOf is when p's failure started, when kwatch noticed it and when
// it first told a channel, from its findings, its open time and the
// delivery trail (oldest first); nil when none is known.
func timingOf(p incident.Incident, trail []DeliveryAttempt) *Timing {
	t := Timing{NoticedAt: p.Opened, StartedAt: startedAt(p)}
	for _, a := range trail {
		if a.Result != deliveredOK || a.At.Before(p.Opened) {
			continue
		}
		if t.ToldAt.IsZero() || a.At.Before(t.ToldAt) {
			t.ToldAt = a.At
			t.ToldVia = channelName(a.Channel)
		}
	}
	if t == (Timing{}) {
		return nil
	}
	return &t
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
