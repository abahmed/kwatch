package announce

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// sendProChecks tells a Community edition with Pro hints how many Pro
// findings it found since it started and did not show. Nothing is sent
// when there are none.
func (c *Collector) sendProChecks(ctx context.Context, now time.Time) {
	if !c.env.Messages.ProNote || c.env.Withheld == nil {
		return
	}
	text := compose.ProChecksText(c.env.Withheld(now))
	if text == "" {
		return
	}
	c.env.Sink(ctx, incident.Decision{Reason: "pro checks"},
		notification.Notice(notification.MarkerLow+" "+text))
}
