package incident

// A page that resolves and fails again is held at notify, so one flapping
// outage pages once (see holdRepeatedPage). The first resolve closed that
// alert, though, and the returns are chat news: an outage that keeps
// coming back would leave the pagers with no open alert at all. So the
// update that says the outage is a flapper, or that it came back for the
// third time in RepageWindow, goes to the pagers once, at the page tier.
// That reopens the alert; the resolve that follows a stable recovery
// closes it.

// pagesFlapper reports an update that tells the pagers a held page keeps
// coming back, and remembers that it did. It is sent once per incident:
// after it the alert is open (Paged), and the flapper does not page again
// each time it resolves and returns.
// Someone who acknowledged the incident knows about it, paging off means
// paging off, and a held page that is half of an outage another incident
// already paged for (pagedBeside) must not open a second alert beside it,
// as releaseHeldPage does not either.
func (m *Manager) pagesFlapper(
	p *Incident, action Action, why Reason,
) bool {
	if action != Update || (why != ReasonFlapping &&
		why != ReasonFailingAgain) {
		return false
	}
	if !p.Delivery.PageHeld() || p.Delivery.OpenAtPagers() ||
		p.Delivery.flapPaged || m.override.noPage {
		return false
	}
	if p.State != Flapping && p.RepeatCount < m.cfg.FlapCycles {
		return false
	}
	if m.pageAcknowledged(p) || m.pagedBeside(p) {
		return false
	}
	p.Delivery.flapPaged = true
	return true
}
