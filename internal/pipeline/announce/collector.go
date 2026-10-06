package announce

import (
	"context"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// Env is everything a Collector needs from the pipeline.
type Env struct {
	Incidents *incident.Manager
	Messages  compose.Writer
	// History is the model's change record; nil without a model.
	History inventory.HistoryReader
	// Scope reports whether people want to hear about an incident; nil
	// keeps every incident.
	Scope func(incident.Incident) bool
	// Sink receives every message with its decision.
	Sink func(context.Context, incident.Decision, notification.Message)
	// Write writes the message for a decision as of a time.
	Write func(incident.Decision, time.Time) notification.Message
	// WithChanges adds the latest changes to a decision of an incident
	// without a cause.
	WithChanges func(incident.Decision, time.Time) incident.Decision
	// SaveStartup hands the startup marker to the incident writer.
	SaveStartup func(StartupState)
	// OwnNamespace is the namespace kwatch runs in, when known: its own
	// risks are left out of the digest.
	OwnNamespace string
}

// Collector holds the state of the collecting steps. Its exported
// fields are that state, readable by the pipeline's tests.
type Collector struct {
	env Env
	// Startup is the cold-start summary state.
	Startup Startup
	// Low collects the digest-tier decisions of the current window.
	Low LowDigest
	// Outages holds each namespace's announcements while an outage opens.
	Outages map[string]*OutageHold

	// advisories lists the active configuration risks; nil when the
	// engine has none to offer. The digest names each once.
	advisories func() []detection.Finding
	// digested are the incidents a digest listed that have not spoken
	// since (see DigestState.Listed).
	digested []string
	// mentionedRisks are the risks a digest already named.
	mentionedRisks map[detection.Key]bool
}

// New returns a Collector over env.
func New(env Env) *Collector {
	return &Collector{
		env:            env,
		Outages:        map[string]*OutageHold{},
		mentionedRisks: map[detection.Key]bool{},
	}
}

// SetAdvisories sets the source of the active configuration risks.
func (c *Collector) SetAdvisories(f func() []detection.Finding) {
	c.advisories = f
}

// NoteDecisions asks the next close check to run when a resolve is among
// decisions, since only a resolve can finish a listing.
func (c *Collector) NoteDecisions(decisions []incident.Decision) {
	for _, d := range decisions {
		if d.Action == incident.Resolve {
			c.Startup.CheckSummary = true
			return
		}
	}
}

// NoteQuietResolves handles the incidents that resolved without a
// decision (a quiet supersede): that is a resolve, so the listings are
// checked, and every announcement still held for them is dropped. They
// told no story of their own, and delivering one later would open a
// thread nobody would ever close.
func (c *Collector) NoteQuietResolves(ids []string) {
	if len(ids) == 0 {
		return
	}
	c.Startup.CheckSummary = true
	gone := func(d incident.Decision) bool {
		return slices.Contains(ids, d.Incident.ID)
	}
	s := &c.Startup
	s.Collected = slices.DeleteFunc(s.Collected, gone)
	c.Low.Opened = slices.DeleteFunc(c.Low.Opened, gone)
	for _, h := range c.Outages {
		h.Held = slices.DeleteFunc(h.Held, gone)
	}
	for _, id := range ids {
		c.env.Incidents.DropAnnouncement(id)
	}
}

// recordCarried hands a held decision to the sink for the audit log only:
// the message names its carrier and delivery drops it. The audit log then
// records every decision when it is made, whether people hear it on its
// own or through the digest or the startup summary.
func (c *Collector) recordCarried(
	ctx context.Context, now time.Time, d incident.Decision, carrier string,
) {
	msg := c.env.Write(d, now)
	msg.Carrier = carrier
	c.env.Sink(ctx, d, msg)
}

// recordDropped is recordCarried for a decision the digest left out. The
// digest tier is meant to be quiet, so this is by design; the note says
// which rule applied, so a reader of the audit log need not guess.
func (c *Collector) recordDropped(
	ctx context.Context, now time.Time, d incident.Decision,
) {
	msg := c.env.Write(d, now)
	msg.Carrier, msg.CarrierNote = CarrierDropped, droppedWhy(d)
	c.env.Sink(ctx, d, msg)
}

// droppedWhy words the rule that left d out of the digest.
func droppedWhy(d incident.Decision) string {
	if d.Action == incident.Resolve {
		return "digest-only: a short blip nobody was told about"
	}
	return "digest-only: an update the digest does not list (" +
		string(d.Reason) + ")"
}

// risesToPage reports an update that took a held incident to the page
// tier when no page reached the pagers for it yet: the hold folded the
// update into its entry, which the pagers never receive, so it is paged
// now. Without it the incident would page nobody until it resolved.
func (c *Collector) risesToPage(d incident.Decision) bool {
	return d.Incident.Tier == incident.Page &&
		!c.env.Incidents.PageOpen(d.Incident.ID)
}

// pageOnly sends d to the paging providers alone and remembers that the
// incident paged, so its resolve reaches them too.
func (c *Collector) pageOnly(
	ctx context.Context, now time.Time, d incident.Decision,
) {
	msg := c.env.Write(d, now)
	msg.PagingOnly = true
	c.env.Incidents.RecordPaged(d.Incident.ID, true)
	c.env.Sink(ctx, d, msg)
}

// closeUnannounced settles a held incident that resolved before anyone was
// told: nobody hears of it, so the held announcement is dropped, not
// delivered. If it had paged, its paging alert is closed with a
// paging-only resolve: the paging providers were told, and without it
// their alert would stay open. carrier names the message that would have
// carried the announcement, for the audit log.
func (c *Collector) closeUnannounced(
	ctx context.Context, now time.Time, d incident.Decision, carrier string,
) {
	c.env.Incidents.DropAnnouncement(d.Incident.ID)
	if !d.Incident.Delivery.OpenAtPagers() {
		c.recordCarried(ctx, now, d, carrier)
		return
	}
	msg := c.env.Write(d, now)
	msg.PagingOnly = true
	c.env.Incidents.RecordPaged(d.Incident.ID, false)
	c.env.Sink(ctx, d, msg)
}
