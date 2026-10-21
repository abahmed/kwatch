package announce

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// An outage keeps spreading after its message: the sixth and the seventh
// workload of a namespace fail a minute or two later, each with no cause
// of its own. Alone they are too few to be an outage (see isOutage), but
// they are not new news either: the namespace already told people. So the
// collector remembers the last outage of each namespace for outageWindow,
// holds the later standalone announcements of that namespace as it holds
// an outage that is forming, and sends them as one more outage message
// that names the whole outage.

// outageMemo is the last outage message of a namespace.
type outageMemo struct {
	// at is when it was sent, or when the latest fold into it was.
	at time.Time
	// members are all the incidents it names, earlier folds included.
	members []incident.Decision
}

// maxOutageMemos bounds the namespaces remembered at once.
const maxOutageMemos = 256

// rememberOutage records that namespace had an outage message at now that
// named members.
func (c *Collector) rememberOutage(
	namespace string, now time.Time, members []incident.Decision,
) {
	if c.outages == nil {
		c.outages = map[string]*outageMemo{}
	}
	for ns, memo := range c.outages {
		if len(c.outages) >= maxOutageMemos ||
			now.Sub(memo.at) > outageWindow {
			delete(c.outages, ns)
		}
	}
	c.outages[namespace] = &outageMemo{at: now, members: members}
}

// recentOutage is the outage message of namespace sent less than
// outageWindow ago, or nil.
func (c *Collector) recentOutage(
	namespace string, now time.Time,
) *outageMemo {
	memo := c.outages[namespace]
	if memo == nil || now.Sub(memo.at) > outageWindow {
		return nil
	}
	return memo
}

// inRecentOutage reports whether namespace had an outage message less
// than outageWindow ago, so a failure of it joins that outage.
func (c *Collector) inRecentOutage(namespace string, now time.Time) bool {
	return c.recentOutage(namespace, now) != nil
}

// outagePageOpen reports whether the alert of an outage that was told is
// open at the pagers: one of the incidents it names still holds it.
func (c *Collector) outagePageOpen(memo *outageMemo) bool {
	for _, d := range memo.members {
		if c.env.Incidents.PageOpen(d.Incident.ID) {
			return true
		}
	}
	return false
}

// holdLatePages holds, at notify, the page-tier members that join an
// outage whose page is open: the namespace was paged for it, and one
// alert stands for the whole outage.
func (c *Collector) holdLatePages(
	memo *outageMemo, members []incident.Decision,
) {
	if !c.outagePageOpen(memo) {
		return
	}
	for i, d := range members {
		if d.Incident.Tier == incident.Page && !c.pagedAlready(d) {
			c.env.Incidents.HoldPage(d.Incident.ID)
			members[i].Incident.Tier = incident.Notify
		}
	}
}
