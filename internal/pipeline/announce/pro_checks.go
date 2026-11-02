package announce

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// proChecksReason is the decision reason of every Pro checks note.
const proChecksReason incident.Reason = "pro checks"

// proChecks remembers the last Pro checks note of this session, so the
// daily note says only what is new and goes out at most once a day.
// Community keeps no startup marker on disk, so this lives in memory: a
// restart may send one more daily note the same day.
type proChecks struct {
	// started is set once the startup note had its turn; the daily
	// note waits for it.
	started bool
	// noted are the counts of noteDay the last note covered.
	noted   feature.WithheldCounts
	noteDay time.Time
	// sentDay is the UTC day the last daily note was sent; zero before.
	sentDay time.Time
}

// sendProChecks tells a Community edition with Pro hints how many Pro
// findings it found since it started and did not show. Nothing is sent
// when there are none.
func (c *Collector) sendProChecks(ctx context.Context, now time.Time) {
	c.markProChecks(now)
	if !c.env.Messages.ProNote || c.env.Withheld == nil {
		return
	}
	text := compose.ProChecksText(c.env.Withheld(now))
	if text == "" {
		return
	}
	c.sendProChecksNote(ctx, text)
}

// markProChecks starts the daily note after the startup one: what was
// counted today until now is what that note already said.
func (c *Collector) markProChecks(now time.Time) {
	c.proChecks.started = true
	c.proChecks.noteDay = utcDay(now)
	if c.env.WithheldToday != nil {
		c.proChecks.noted = c.env.WithheldToday(now)
	}
}

// SendDailyProChecks sends, at most once per UTC day and only after the
// startup note, "Pro checks found N issues today: ..." when Pro findings
// were withheld today and some are new since the last note. It is silent
// without Pro hints (app.hideProHints) and when there is nothing new.
func (c *Collector) SendDailyProChecks(ctx context.Context, now time.Time) {
	s := &c.proChecks
	if !c.env.Messages.ProNote || c.env.WithheldToday == nil ||
		!s.started {
		return
	}
	day := utcDay(now)
	if s.sentDay.Equal(day) {
		return
	}
	counts := c.env.WithheldToday(now)
	if counts.Total() == 0 ||
		(s.noteDay.Equal(day) && counts == s.noted) {
		return
	}
	c.sendProChecksNote(ctx, compose.ProChecksTodayText(counts))
	s.noted, s.noteDay, s.sentDay = counts, day, day
}

// sendProChecksNote sends text as a low-tier notice: it never pages.
func (c *Collector) sendProChecksNote(ctx context.Context, text string) {
	c.env.Sink(ctx, incident.Decision{Reason: proChecksReason},
		notification.Notice(notification.MarkerLow+" "+text))
}

// utcDay is the start of the UTC day of t.
func utcDay(t time.Time) time.Time {
	return t.UTC().Truncate(24 * time.Hour)
}
