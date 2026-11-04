package incident

// Told reports an incident some message reached: an announcement, an
// update or a page that is still open at the pagers. A restored record
// keeps it (Record.Unheard).
func (p Incident) Told() bool {
	return p.sent.told || p.Delivery.OpenAtPagers()
}

// Untold reports an incident that was never announced and that no
// message reached: it settled, recovered or moved before alerting. The
// dashboard shows it as such and counts it as no page or notification.
func (p Incident) Untold() bool {
	return p.Announced.IsZero() && !p.Told()
}

// DigestBound reports an open incident whose live findings only wait
// for the digest: an idle fail-closed webhook that refused nothing, or
// failures that are all digest findings. It was told once already, so a
// restart lists it with the lower-priority ones instead of announcing
// it again. A held page never is.
func (p Incident) DigestBound() bool {
	if len(p.Members) == 0 || p.Delivery.PageHeld() || p.criticalRoot() {
		return false
	}
	return idleWebhook(&p) || quietMembers(&p)
}
