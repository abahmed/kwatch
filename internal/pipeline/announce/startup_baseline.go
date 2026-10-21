package announce

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// baselineMark is how a baselined incident looked when it was absorbed.
type baselineMark struct {
	tier     incident.Tier
	severity detection.Severity
	members  int
}

func markOf(p incident.Incident) baselineMark {
	m := baselineMark{tier: p.Tier, members: len(p.Members)}
	for _, f := range p.Members {
		m.severity = max(m.severity, f.Severity)
	}
	return m
}

// worseThan reports whether m is more severe or larger than base.
func (m baselineMark) worseThan(base baselineMark) bool {
	return m.tier > base.tier || m.severity > base.severity ||
		m.members > base.members
}

// baseline records that incident p existed at start, with its severity
// and member count.
func (s *Startup) baseline(p incident.Incident) {
	if s.baselined == nil {
		s.baselined = map[string]baselineMark{}
	}
	s.baselined[p.ID] = markOf(p)
}

// Baselined reports whether incident id existed at start and nobody was
// told about it.
func (s *Startup) Baselined(id string) bool {
	_, ok := s.baselined[id]
	return ok
}

// absorbBaselined drops the decisions about incidents that existed at
// start, for the audit log only, and returns the rest. A resolve ends the
// incident's place in the baseline. An update that raises the severity or
// grows the member count beyond the baseline is let through, once: the
// incident then leaves the baseline and behaves like a normal one. As
// nobody was told of it, that first message is a full announcement.
func (c *Collector) absorbBaselined(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) []incident.Decision {
	s := &c.Startup
	if len(s.baselined) == 0 {
		return decisions
	}
	var rest []incident.Decision
	for _, d := range decisions {
		id := d.Incident.ID
		base, ok := s.baselined[id]
		switch {
		case !ok:
			rest = append(rest, d)
		case d.Action == incident.Resolve:
			delete(s.baselined, id)
			c.closeUnannounced(ctx, now, d, "baseline")
		case d.Action != incident.Announce &&
			markOf(d.Incident).worseThan(base):
			delete(s.baselined, id)
			d.Action = incident.Announce
			rest = append(rest, d)
		default:
			c.recordCarried(ctx, now, d, "baseline")
		}
	}
	return rest
}
