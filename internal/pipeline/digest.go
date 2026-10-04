package pipeline

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// digestWindow is how long low-tier news is collected before one digest
// message carries all of it. Digest-tier incidents are worth knowing, not
// worth an interruption each: an autoscaler at its maximum, a budget that
// selects nothing, a container throttled on CPU.
const digestWindow = 30 * time.Minute

// lowDigest collects the digest-tier decisions of the current window.
type lowDigest struct {
	// opened are the announcements not yet sent, each at its newest
	// revision.
	opened []incident.Decision
	// resolved are the resolves of incidents an earlier digest listed.
	resolved []incident.Decision
	// since is when the first pending entry arrived; zero while none.
	since time.Time
}

// collectDigest holds digest-tier decisions and sends them as one message
// once the window is over. An announcement held here is marked held, so a
// restart announces it again instead of losing it. An update of a
// digest-tier incident is never sent on its own: the next digest carries
// the newest state. A resolve of an incident nobody heard of yet is
// dropped with its announcement; one of an incident an earlier digest
// listed goes into the next digest. An incident that leaves the digest
// tier before its digest is announced at once: that is news. It reports
// whether a digest was sent in this call.
func (a *announcer) collectDigest(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	rest := make([]incident.Decision, 0, len(decisions))
	for _, d := range decisions {
		if out, held := a.holdDigest(now, d); !held {
			rest = append(rest, out)
		}
	}
	return rest, a.flushDigest(ctx, now)
}

// holdDigest reports whether d was absorbed by the digest. A decision
// that passes through may come back changed: the first message of an
// incident promoted out of the digest is written as an announcement.
func (a *announcer) holdDigest(
	now time.Time, d incident.Decision,
) (incident.Decision, bool) {
	g := &a.digest
	id := d.Incident.ID
	i := indexOfIncident(g.opened, id)
	if d.Incident.Tier != incident.Digest {
		if i >= 0 {
			g.opened = append(g.opened[:i], g.opened[i+1:]...)
			a.incidents.ReleaseAnnouncement(id)
			if d.Action == incident.Update {
				d.Action = incident.Announce
			}
		}
		return d, false
	}
	switch d.Action {
	case incident.Announce:
		if i >= 0 {
			g.opened[i] = d
		} else {
			g.opened = append(g.opened, d)
			a.incidents.HoldAnnouncement(id)
		}
	case incident.Update:
		if i >= 0 {
			g.opened[i].Incident = d.Incident
		}
	case incident.Resolve:
		if i >= 0 {
			// Resolved before anyone was told: leave it out.
			g.opened = append(g.opened[:i], g.opened[i+1:]...)
			a.incidents.ReleaseAnnouncement(id)
			return d, true
		}
		g.resolved = append(g.resolved, d)
	default:
		return d, false
	}
	if g.since.IsZero() {
		g.since = now
	}
	return d, true
}

// flushDigest sends the pending digest once its window is over, and
// reports whether it did.
func (a *announcer) flushDigest(ctx context.Context, now time.Time) bool {
	g := &a.digest
	if g.since.IsZero() || now.Before(g.since.Add(digestWindow)) {
		return false
	}
	sent := false
	if len(g.opened)+len(g.resolved) > 0 {
		msg := a.messages.Digest(g.opened, g.resolved, now)
		a.sink(ctx, incident.Decision{Reason: "digest"}, msg)
		for _, d := range g.opened {
			a.incidents.ReleaseAnnouncement(d.Incident.ID)
		}
		sent = true
	}
	*g = lowDigest{}
	return sent
}

// nextDigest is when the pending digest is due, or zero when none is.
func (a *announcer) nextDigest() time.Time {
	if a.digest.since.IsZero() {
		return time.Time{}
	}
	return a.digest.since.Add(digestWindow)
}

// indexOfIncident returns the position of incident id in decisions, or
// -1.
func indexOfIncident(decisions []incident.Decision, id string) int {
	for i, d := range decisions {
		if d.Incident.ID == id {
			return i
		}
	}
	return -1
}
