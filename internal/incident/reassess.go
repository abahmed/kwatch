package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// Lowering the tier. An announced incident keeps the loudest tier it
// reached (ratchetTier), because crash-loop members come and go within
// seconds and a tier that followed them would flip. One change is not
// churn: the findings the tier stood on are gone and what is left only
// waits for the digest, as when a better detector words the same problem
// as an informational finding. The incident then falls to the digest
// tier and tells its thread the cause once, through the cause-revised
// update. A page whose paging failures all cleared falls the same way;
// it is never paged again, and a rise is still told by the material
// change.

// quietMembers reports an incident whose failing members are all of the
// kind that only waits for the digest, and that has at least one.
// Configuration risks do not count either way.
func quietMembers(p *Incident) bool {
	failing := false
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		failing = true
		if !waitsForDigest(s) && s.Severity > detection.Info {
			return false
		}
	}
	return failing
}

// lowerable reports an open incident that should fall to the digest:
// only quiet members are left, the restore grace is over so that
// members still coming back are not missed, and the last update was not
// within MaterialGap. A page falls too once every failure that paged
// has cleared and only digest findings are left (a pressure stall that
// outlived a crash loop): it is not paged again, and its open alert
// closes with the resolve. An idle webhook page keeps its own rule.
func (m *Manager) lowerable(p *Incident, now time.Time) bool {
	if p.State != Open || !m.fallsQuiet(p) ||
		m.inGrace(p, now) || drainingRoot(p) ||
		m.override.apply(p, Digest) >= p.Tier {
		return false
	}
	last := p.Delivery.LastMaterial()
	return last.IsZero() || !now.Before(last.Add(MaterialGap))
}

// fallsQuiet reports an incident whose tier the digest can carry: a
// notification, or a page, left with only quiet members, or a
// notification about an idle webhook.
func (m *Manager) fallsQuiet(p *Incident) bool {
	switch {
	case p.Tier == Page:
		return quietMembers(p) && !p.criticalRoot()
	case p.Tier != Notify || p.Delivery.PageHeld():
		return false
	}
	return quietMembers(p) || idleWebhook(p) || chronicFalls(p)
}

// reassess lowers the tier of p when lowerable says so, and owes the
// thread one cause-revised update, sent after the revise settle like
// any other. The tier is lowered before the update is decided, so the
// update is the only message of the change.
func (m *Manager) reassess(p *Incident, now time.Time) {
	if !m.lowerable(p, now) {
		return
	}
	p.Tier = m.override.apply(p, Digest)
	p.Delivery.MarkDemoted()
	if idleWebhook(p) || chronicFalls(p) {
		// Told once at the announcement; the digest carries it now,
		// and the new tier is no news for the thread.
		p.Digest = fingerprint(p)
		return
	}
	p.Delivery.MarkMaterial(now)
	p.Pending.MarkRevised(now)
	p.note(now, "cause revised: only digest findings are left")
}

// reassessDeadline is when p next needs a tick to be judged: when the
// restore grace of an incident that may fall is over, or when the
// spacing of its last update is.
func (m *Manager) reassessDeadline(
	p *Incident, now time.Time,
) (time.Time, bool) {
	if p.State != Open || !m.fallsQuiet(p) {
		return time.Time{}, false
	}
	if m.inGrace(p, now) {
		return m.restoreGrace, true
	}
	if last := p.Delivery.LastMaterial(); !last.IsZero() &&
		now.Before(last.Add(MaterialGap)) {
		return last.Add(MaterialGap), true
	}
	return time.Time{}, false
}
