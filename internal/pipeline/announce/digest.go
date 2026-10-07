package announce

import (
	"context"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// DigestWindow is how long low-tier news is collected before one digest
// message carries all of it. Digest-tier incidents are worth knowing, not
// worth an interruption each: an autoscaler at its maximum, a budget that
// selects nothing, a container throttled on CPU.
const DigestWindow = 30 * time.Minute

// LowDigest collects the digest-tier decisions of the current window.
type LowDigest struct {
	// Opened are the announcements not yet sent, each at its newest
	// revision.
	Opened []incident.Decision
	// Resolved are the resolves of incidents an earlier digest listed.
	Resolved []incident.Decision
	// Since is when the first pending entry arrived; zero while none.
	Since time.Time
}

// CollectDigest holds digest-tier decisions and sends them as one message
// once the window is over. An announcement held here is marked held, so a
// restart announces it again instead of losing it. An update of a
// digest-tier incident is never sent on its own: the next digest carries
// the newest state. A resolve of an incident nobody heard of yet is
// dropped with its announcement; one of an incident an earlier digest
// listed goes into the next digest. An incident that leaves the digest
// tier before its digest is announced at once: that is news; so is the
// new cause of one that fell to it from a thread (see threadNews). It reports
// whether a digest was sent in this call.
func (c *Collector) CollectDigest(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	rest := make([]incident.Decision, 0, len(decisions))
	for _, d := range decisions {
		out, carrier := c.holdDigest(now, d)
		if carrier == "" {
			rest = append(rest, out)
			continue
		}
		if carrier == CarrierDropped {
			c.recordDropped(ctx, now, d)
			continue
		}
		c.recordCarried(ctx, now, d, carrier)
	}
	c.noteWake(now)
	c.noteOngoing(now)
	sent := c.flushDigest(ctx, now)
	c.saveDigest()
	return rest, sent
}

// maxMentionedRisks bounds the memory of named workloads; past it the
// digest may name old ones again, which costs one line each. They ride
// along a digest that goes out anyway: they never cost a message of
// their own.
const maxMentionedRisks = 4096

// PendingRisks lists the active workloads whose pods run but never
// become ready, which no digest has named yet, in a stable order. That
// is the only advisory finding announced: it is happening now. Advice
// such as a single replica or a missing probe is never listed.
func (c *Collector) PendingRisks() []detection.Finding {
	if c.advisories == nil {
		return nil
	}
	var out []detection.Finding
	for _, f := range c.advisories() {
		if f.Reason == reasons.WorkloadNeverReady &&
			!c.mentionedRisks[f.Key()] {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entity != out[j].Entity {
			return out[i].Entity.String() < out[j].Entity.String()
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// mentionRisks remembers that a digest named these risks.
func (c *Collector) mentionRisks(risks []detection.Finding) {
	if len(c.mentionedRisks) > maxMentionedRisks {
		c.mentionedRisks = map[detection.Key]bool{}
	}
	for _, f := range risks {
		c.mentionedRisks[f.Key()] = true
	}
}

// holdDigest absorbs d into the digest and names what carries it for the
// audit log: "digest" when a digest line carries it, CarrierDropped when
// the digest took it in and left it out, and "" when d was not absorbed.
// A decision that passes through may come back changed: the first message
// of an incident promoted out of the digest is written as an
// announcement.
func (c *Collector) holdDigest(
	now time.Time, d incident.Decision,
) (incident.Decision, string) {
	g := &c.Low
	id := d.Incident.ID
	i := indexOfIncident(g.Opened, id)
	if d.Incident.Tier != incident.Digest {
		return c.promoteFromDigest(g, i, d), ""
	}
	if threadNews(d) {
		return d, ""
	}
	carrier := "digest"
	switch d.Action {
	case incident.Announce:
		c.digestAnnounce(g, i, d)
	case incident.Update:
		if !digestUpdate(g, i, d) {
			carrier = CarrierDropped
		}
	case incident.Resolve:
		if i >= 0 {
			if !c.resolveInDigest(g, i, d, now) {
				carrier = CarrierDropped
			}
			return d, carrier
		}
		g.Resolved = append(g.Resolved, d)
	default:
		return d, ""
	}
	if g.Since.IsZero() {
		g.Since = now
	}
	return d, carrier
}

// threadNews reports a decision of an incident that fell to the digest
// tier after its own thread was opened above it: the new cause and the
// resolve go to that thread, which a digest line would leave open. Its
// other updates, reminders among them, ride in the digest.
func threadNews(d incident.Decision) bool {
	return d.Thread && (d.Action == incident.Resolve ||
		(d.Action == incident.Update &&
			d.Reason == incident.ReasonCauseRevised))
}

// promoteFromDigest takes an incident that left the digest tier out of the
// pending digest, so it is announced at once: that is news.
//
// An incident an earlier digest listed has not been introduced either: its
// first message of its own becomes an announcement, as for the listings of
// a summary or a roll-up (see Listing.follow), so a thread, an alert or an
// issue opens with the full story and not with a bare update.
func (c *Collector) promoteFromDigest(
	g *LowDigest, i int, d incident.Decision,
) incident.Decision {
	if c.takeListed(d.Incident.ID) && d.Action == incident.Update {
		d.Action = incident.Announce
	}
	if i < 0 {
		return d
	}
	g.Opened = append(g.Opened[:i], g.Opened[i+1:]...)
	c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
	if d.Action == incident.Update {
		d.Action = incident.Announce
	}
	return d
}

// digestAnnounce adds an announcement to the pending digest, or replaces
// its older revision.
func (c *Collector) digestAnnounce(g *LowDigest, i int, d incident.Decision) {
	if i >= 0 {
		g.Opened[i] = d
		return
	}
	g.Opened = append(g.Opened, d)
	c.env.Incidents.HoldAnnouncement(d.Incident.ID)
}

// digestUpdate folds an update into the pending entry. A reminder of a
// long-open digest incident no earlier digest will list again ("still
// failing, for 3d now") is added, and so is the return of a resolved
// digest incident ("failing again"): people already heard of it, so it is
// not held on its incident; the digest state keeps them (see
// DigestState) so a restart loses nothing. A return replaces the resolve
// the digest may still hold for it: it came back, so that is not news.
// It reports whether the digest carries the update; any other update of
// an incident the digest does not hold has nothing to list and is dropped.
func digestUpdate(g *LowDigest, i int, d incident.Decision) bool {
	switch {
	case i >= 0:
		g.Opened[i].Incident = d.Incident
	case d.Reason == incident.ReasonReminder:
		g.Opened = append(g.Opened, d)
	case d.Reason == incident.ReasonFailingAgain:
		if j := indexOfIncident(g.Resolved, d.Incident.ID); j >= 0 {
			g.Resolved = append(g.Resolved[:j], g.Resolved[j+1:]...)
		}
		g.Opened = append(g.Opened, d)
	default:
		return false
	}
	return true
}

// resolveInDigest settles a digest entry whose incident resolved before
// the digest went out. A first blip nobody was told about is left out.
// A reminder entry was told earlier, and a recurrence is worth a line
// (see incident.DigestWorthy): both move to the resolved list, so
// something that comes back after each resolve still shows up. It
// reports whether the resolve went into the digest; false means dropped.
func (c *Collector) resolveInDigest(
	g *LowDigest, i int, d incident.Decision, now time.Time,
) bool {
	told := g.Opened[i].Reason == incident.ReasonReminder ||
		g.Opened[i].Reason == incident.ReasonFailingAgain
	g.Opened = append(g.Opened[:i], g.Opened[i+1:]...)
	if told || incident.DigestWorthy(d.Incident, now) {
		c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
		g.Resolved = append(g.Resolved, d)
		return true
	}
	c.env.Incidents.DropAnnouncement(d.Incident.ID)
	return false
}

// flushDigest sends the pending digest once its window is over, and
// reports whether it did.
func (c *Collector) flushDigest(ctx context.Context, now time.Time) bool {
	g := &c.Low
	if g.Since.IsZero() || now.Before(g.Since.Add(DigestWindow)) {
		return false
	}
	sent := false
	ongoing := c.ongoingProblems()
	if len(g.Opened)+len(g.Resolved)+len(ongoing) > 0 || c.wake != nil {
		risks := c.PendingRisks()
		msg := c.env.Messages.DigestWith(g.Opened, g.Resolved, risks,
			c.digestExtras(ongoing), now)
		msg.Listed = c.listedInDigest(g, len(risks), ongoing, now)
		c.env.Sink(ctx, incident.Decision{Reason: "digest"}, msg)
		for _, d := range g.Opened {
			c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
			c.env.Incidents.RecordDigested(d.Incident.ID, now)
			c.markListed(d.Incident.ID)
			c.markSeen(d.Incident)
		}
		for _, d := range g.Resolved {
			c.env.Incidents.RecordDigested(d.Incident.ID, now)
			c.takeListed(d.Incident.ID)
		}
		for _, item := range ongoing {
			c.env.Incidents.RecordDigested(item.p.ID, now)
			c.markSeen(item.p)
		}
		c.mentionRisks(risks)
		c.wake = nil
		sent = true
	}
	*g = LowDigest{}
	return sent
}

// digestExtras is what the digest says beyond its incidents.
func (c *Collector) digestExtras(ongoing []ongoingItem) compose.DigestExtras {
	extras := compose.DigestExtras{Wake: c.wake}
	for _, item := range ongoing {
		extras.Ongoing = append(extras.Ongoing, item.Ongoing)
	}
	return extras
}

// MaxAuditItems bounds the incidents a digest's audit entry names.
const MaxAuditItems = 20

// listedInDigest summarises a digest for the audit log: the counts, and
// the first MaxAuditItems incidents as "id: title".
func (c *Collector) listedInDigest(
	g *LowDigest, risks int, ongoing []ongoingItem, now time.Time,
) *notification.Listed {
	listed := &notification.Listed{
		Opened: len(g.Opened), Resolved: len(g.Resolved), Risks: risks,
		Ongoing: len(ongoing),
	}
	add := func(decisions []incident.Decision, suffix string) {
		for _, d := range decisions {
			if len(listed.Items) == MaxAuditItems {
				return
			}
			listed.Items = append(listed.Items, d.Incident.ID+": "+
				c.env.Messages.Write(d, now).Title+suffix)
		}
	}
	add(g.Opened, "")
	add(g.Resolved, " (resolved)")
	for _, item := range ongoing {
		if len(listed.Items) < MaxAuditItems {
			listed.Items = append(listed.Items,
				item.p.ID+": "+item.Line(now)+" (ongoing)")
		}
	}
	return listed
}

// NextDigest is when the pending digest is due, or zero when none is.
func (c *Collector) NextDigest() time.Time {
	if c.Low.Since.IsZero() {
		return time.Time{}
	}
	return c.Low.Since.Add(DigestWindow)
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
