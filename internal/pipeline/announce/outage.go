package announce

import (
	"context"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// OutageHold is the announcements of one namespace waiting for the rest
// of an outage, in decision order.
type OutageHold struct {
	Since time.Time
	Held  []incident.Decision
	// Paged is true once one held page-tier incident was paged: the
	// namespace is paged once, whoever else is page tier. PagedID is
	// that incident.
	Paged   bool
	PagedID string
}

func (h *OutageHold) indexOf(id string) int {
	for i, d := range h.Held {
		if d.Incident.ID == id {
			return i
		}
	}
	return -1
}

// CollectOutages holds the announcements of a namespace that is having
// an outage, then sends them as one message once the namespace has no
// incident left settling or outageHoldMax passed. It is a
// namespace-scoped roll-up: the members keep their own conversations,
// which thread under the message, and the roll-up resolve closes it. It
// returns the decisions to go on with and whether it sent a message.
func (c *Collector) CollectOutages(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) ([]incident.Decision, bool) {
	var rest []incident.Decision
	for _, d := range decisions {
		if !c.HoldOutage(ctx, now, d) {
			rest = append(rest, d)
		}
	}
	released, sent := c.flushOutages(ctx, now)
	return append(rest, released...), sent
}

// HoldOutage reports whether d was absorbed by a namespace's hold. A
// page-tier announcement still reaches the paging tools at once, as the
// startup summary does, so the hold never delays a page.
func (c *Collector) HoldOutage(
	ctx context.Context, now time.Time, d incident.Decision,
) bool {
	ns, id := d.Incident.Root.Namespace, d.Incident.ID
	h := c.Outages[ns]
	if isOutageCandidate(d) && (h != nil || c.outageForming(d, now)) {
		if h == nil {
			h = &OutageHold{Since: now}
			c.Outages[ns] = h
		}
		c.absorbAnnounce(ctx, now, h, d)
		return true
	}
	i := -1
	if h != nil {
		i = h.indexOf(id)
	}
	switch {
	case i < 0:
		return false
	case d.Action == incident.Update:
		c.absorbUpdate(ctx, now, h, i, d)
	case d.Action == incident.Resolve:
		c.absorbResolve(ctx, now, h, i, d)
	default:
		c.recordCarried(ctx, now, d, "roll-up")
	}
	return true
}

// absorbAnnounce holds a candidate's announcement, replacing an earlier
// one of the same incident, and pages for the first page-tier member.
func (c *Collector) absorbAnnounce(
	ctx context.Context, now time.Time, h *OutageHold, d incident.Decision,
) {
	c.env.Incidents.HoldAnnouncement(d.Incident.ID)
	if i := h.indexOf(d.Incident.ID); i >= 0 {
		h.Held[i] = d
	} else {
		h.Held = append(h.Held, d)
	}
	if d.Incident.Tier == incident.Page && !h.Paged {
		c.pageOnly(ctx, now, d)
		h.Paged, h.PagedID = true, d.Incident.ID
	}
}

// absorbUpdate keeps the newest state of a held incident; the update
// itself is carried by the roll-up message.
func (c *Collector) absorbUpdate(
	ctx context.Context, now time.Time, h *OutageHold, i int,
	d incident.Decision,
) {
	h.Held[i].Incident = d.Incident
	if !h.Paged && c.risesToPage(d) {
		// The namespace is paged once: this incident stands for it.
		c.pageOnly(ctx, now, d)
		h.Paged, h.PagedID = true, d.Incident.ID
		return
	}
	c.recordCarried(ctx, now, d, "roll-up")
}

// absorbResolve drops a held incident that resolved before anyone was
// told, closing its paging alert if it had paged (see closeUnannounced).
func (c *Collector) absorbResolve(
	ctx context.Context, now time.Time, h *OutageHold, i int,
	d incident.Decision,
) {
	h.Held = append(h.Held[:i], h.Held[i+1:]...)
	c.closeUnannounced(ctx, now, d, "roll-up")
}

// outageForming reports whether d is one of an outage that is still
// opening: counting d and the incidents of its namespace that are
// settling and have no cause yet, there are outageIncidents. A smaller
// group is held only after one is already held: waiting for three
// workloads of a small namespace would delay every failure there.
func (c *Collector) outageForming(
	d incident.Decision, now time.Time,
) bool {
	pending := 1 + c.settlingIn(d.Incident.Root.Namespace, now)
	return pending >= outageIncidents
}

// settlingIn counts the incidents of a namespace that are still
// settling, would be outage members and opened within the window.
func (c *Collector) settlingIn(namespace string, now time.Time) int {
	return c.env.Incidents.SettlingUnexplained(
		namespace, now.Add(-outageWindow))
}

// flushOutages sends the hold of every namespace whose outage has
// settled. Incidents that turn out not to be an outage are returned, to
// be announced one by one.
func (c *Collector) flushOutages(
	ctx context.Context, now time.Time,
) ([]incident.Decision, bool) {
	var released []incident.Decision
	sent := false
	for _, ns := range c.outageNamespaces() {
		h := c.Outages[ns]
		waiting := c.settlingIn(ns, now) > 0
		if waiting && now.Before(h.Since.Add(outageHoldMax)) {
			continue
		}
		delete(c.Outages, ns)
		members := openedTogether(h.Held)
		if !isOutage(len(members), c.workloadCount(ns)) {
			members = nil
		}
		for _, d := range h.Held {
			if !containsDecision(members, d) {
				released = append(released, c.release(h, d))
			}
		}
		if len(members) > 0 {
			c.sendOutage(ctx, now, ns, members)
			sent = true
		}
	}
	return released, sent
}

// release hands a held announcement back to normal delivery. Delivery
// has it from now on, so it is no longer held. If it is the incident the
// hold already paged, the announcement is marked so the pagers are not
// sent it a second time.
func (c *Collector) release(
	h *OutageHold, d incident.Decision,
) incident.Decision {
	c.env.Incidents.ReleaseAnnouncement(d.Incident.ID)
	d.PagedAlready = d.PagedAlready ||
		(h.Paged && h.PagedID == d.Incident.ID)
	return d
}

// sendOutage sends the one message that names the members.
func (c *Collector) sendOutage(
	ctx context.Context, now time.Time, namespace string,
	members []incident.Decision,
) {
	o := compose.NamespaceOutage{Namespace: namespace,
		Workloads: c.workloadCount(namespace),
		Shared:    c.sharedBy(members, now)}
	msg := c.env.Messages.NamespaceOutage(o, members, now)
	c.sendListing(ctx, now, members, msg, true)
}

// outageNamespaces lists the namespaces with a hold, in order.
func (c *Collector) outageNamespaces() []string {
	names := make([]string, 0, len(c.Outages))
	for ns := range c.Outages {
		names = append(names, ns)
	}
	sort.Strings(names)
	return names
}

func containsDecision(ds []incident.Decision, d incident.Decision) bool {
	for _, other := range ds {
		if other.Incident.ID == d.Incident.ID {
			return true
		}
	}
	return false
}

// NextOutage is when the oldest hold is sent at the latest, or zero.
func (c *Collector) NextOutage() time.Time {
	var next time.Time
	for _, h := range c.Outages {
		if at := h.Since.Add(outageHoldMax); next.IsZero() ||
			at.Before(next) {
			next = at
		}
	}
	return next
}
