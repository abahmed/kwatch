package announce

import (
	"context"
	"slices"
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

// baselineEntry is a baselined incident: how it looked at start, and
// its announcement, kept with the incident's latest state so it can be
// sent when the incident is still failing after the startup window.
type baselineEntry struct {
	mark baselineMark
	held incident.Decision
}

// baseline records that the incident d announces existed at start,
// with its severity and member count.
func (s *Startup) baseline(d incident.Decision) {
	if s.baselined == nil {
		s.baselined = map[string]baselineEntry{}
	}
	s.baselined[d.Incident.ID] = baselineEntry{mark: markOf(d.Incident),
		held: d}
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
// incident then leaves the baseline and behaves like a normal one. So is
// any update once the startup window is over: the incident failed again
// after it was left in the baseline as recovering (a relapse that makes no
// decision is found by sweepBaselined). As nobody was told of it, that
// first message is a full announcement.
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
			(s.baselineOver || markOf(d.Incident).worseThan(base.mark)):
			delete(s.baselined, id)
			d.Action = incident.Announce
			rest = append(rest, d)
		default:
			base.held.Incident = d.Incident
			s.baselined[id] = base
			c.recordCarried(ctx, now, d, "baseline")
		}
	}
	return rest
}

// releaseBaselined ends the baseline when the startup window closes:
// every baselined incident that is still failing is announced as a new
// incident would be, oldest first, and goes through the digest, outage
// and roll-up steps like any other announcement. A failure that
// recovered inside the window was resolved quietly already; one that is
// recovering stays in the baseline, so its resolve is quiet too and a
// relapse is announced by a later sweep (see sweepBaselined).
func (c *Collector) releaseBaselined() []incident.Decision {
	c.Startup.baselineOver = true
	return c.sweepBaselined()
}

// sweepBaselined announces, once and oldest first, every baselined
// incident whose live state is open or flapping, and drops the ones that
// are gone or resolved. It runs when the window closes and on every tick
// after: an incident that fails again while it is recovering stays the
// same incident and makes no decision of its own, so only its live state
// shows the relapse.
func (c *Collector) sweepBaselined() []incident.Decision {
	s := &c.Startup
	var out []incident.Decision
	for id, entry := range s.baselined {
		d := entry.held
		if c.env.Incidents != nil {
			p := c.env.Incidents.Incident(id)
			if p == nil || p.State == incident.Resolved {
				delete(s.baselined, id)
				continue
			}
			d.Incident = *p
		}
		if d.Incident.State != incident.Open &&
			d.Incident.State != incident.Flapping {
			continue
		}
		delete(s.baselined, id)
		d.Action = incident.Announce
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b incident.Decision) int {
		if c := a.Incident.Opened.Compare(b.Incident.Opened); c != 0 {
			return c
		}
		return compareIDs(a.Incident.ID, b.Incident.ID)
	})
	return out
}

func compareIDs(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
