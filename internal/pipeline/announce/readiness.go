package announce

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/status"
)

// readinessEvery is the shortest time between two readiness items.
const readinessEvery = 24 * time.Hour

// readinessItem is the upgrade-readiness line a digest carries when the
// answer changed. It rides along a digest that goes out anyway and never
// costs a message of its own. Its memory is not saved: after a restart
// the next digest may repeat the line once.
type readinessItem struct {
	source func() status.Readiness
	// lastKey is what the last line said; lastAt when it was sent.
	lastKey string
	lastAt  time.Time
}

// SetReadiness sets the source of the upgrade-readiness answer.
func (c *Collector) SetReadiness(f func() status.Readiness) {
	c.readiness.source = f
}

// attach adds the readiness line to a digest message when there are
// blockers, they differ from the last line and a day has passed.
// Whatever a listed incident already says is left out.
func (r *readinessItem) attach(
	msg *notification.Message, g *LowDigest, now time.Time,
) {
	if r.source == nil {
		return
	}
	if !r.lastAt.IsZero() && now.Sub(r.lastAt) < readinessEvery {
		return
	}
	answer := r.source().Skipping(listedEntities(g))
	if answer.Total == 0 {
		r.lastKey = ""
		return
	}
	if answer.Key() == r.lastKey {
		return
	}
	line := answer.Line()
	msg.Lines = append(msg.Lines, line)
	msg.Note += " " + line
	msg.Doc = append(msg.Doc, notification.Block{Kind: notification.Para,
		Spans: []notification.Span{{Text: line}}})
	r.lastKey, r.lastAt = answer.Key(), now
}

// listedEntities are the entities the digest's incidents are about.
func listedEntities(g *LowDigest) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, list := range [][]incident.Decision{g.Opened, g.Resolved} {
		for _, d := range list {
			out[d.Incident.Root] = true
			for key := range d.Incident.Members {
				out[key.Entity] = true
			}
		}
	}
	return out
}
