package incident

import "time"

// Delivery is what people have been told about an incident and how it
// reached them. Its fields change only through the methods below, so
// each flag has one writer to look for. Export and restored
// (persist.go) keep paged, pageHeld, rolledUp and podPeak. sentTier,
// sentGrowth and lastMaterial are not kept: after a restart the first
// material update is not spaced by MaterialGap, and the sent size is
// adopted again (adoptFingerprints, flapGrew), so that costs no message.
type Delivery struct {
	// paged: the alert is open at the paging providers.
	paged bool
	// pageHeld: a repeated page was suppressed, so it notifies instead.
	pageHeld bool
	// rolledUp: a roll-up or the startup summary carried the announcement.
	rolledUp bool
	// podPeak: most distinct pods failing at once since the roll-up.
	podPeak int
	// sentTier is the tier of the latest decision.
	sentTier Tier
	// sentGrowth is growthKey at the latest decision.
	sentGrowth string
	// lastMaterial is when the last material-change update was decided.
	lastMaterial time.Time
}

// OpenAtPagers reports an alert open at the paging providers.
func (d *Delivery) OpenAtPagers() bool { return d.paged }

// MarkPaged records that a message of its own opened the alert, so its
// resolve must reach the pagers too.
func (d *Delivery) MarkPaged() { d.paged = true }

// ClosePage records that the resolve closed the alert, so nothing more
// goes to the pagers.
func (d *Delivery) ClosePage() { d.paged = false }

// PageHeld reports a repeated page that notifies instead of paging.
func (d *Delivery) PageHeld() bool { return d.pageHeld }

// HoldAtNotify records a suppressed repeated page; never cleared, so the
// incident keeps the page's reminders and history.
func (d *Delivery) HoldAtNotify() { d.pageHeld = true }

// RolledUp reports an announcement a roll-up or summary carried.
func (d *Delivery) RolledUp() bool { return d.rolledUp }

// MarkRolledUp records the carry and adopts the current pod count, so
// the adoption itself does not read as growth.
func (d *Delivery) MarkRolledUp(pods int) {
	d.rolledUp, d.podPeak = true, pods
}

// PodPeak is the most pods that failed at once since the roll-up.
func (d *Delivery) PodPeak() int { return d.podPeak }

// RaisePodPeak keeps the peak growing only, so the fingerprint sees
// growth and never a dip.
func (d *Delivery) RaisePodPeak(pods int) { d.podPeak = max(d.podPeak, pods) }

// SentTier is the tier people were last told.
func (d *Delivery) SentTier() Tier { return d.sentTier }

// SentGrowth is the failure size people were last told.
func (d *Delivery) SentGrowth() string { return d.sentGrowth }

// RecordSent remembers what a decision told people, to tell news from
// churn at the next one.
func (d *Delivery) RecordSent(tier Tier, growth string) {
	d.sentTier, d.sentGrowth = tier, growth
}

// AdoptGrowth sets the remembered size when none is known (a restored
// incident), so it says nothing until the failure outgrows it.
func (d *Delivery) AdoptGrowth(growth string) { d.sentGrowth = growth }

// LastMaterial is when the last material-change update was decided.
func (d *Delivery) LastMaterial() time.Time { return d.lastMaterial }

// MarkMaterial spaces material updates by MaterialGap.
func (d *Delivery) MarkMaterial(now time.Time) { d.lastMaterial = now }

// Pending is follow-up work an incident still owes its thread. Fields
// change only through the methods below.
type Pending struct {
	// reopenedAt: a resolved page came back; its "failing again" update
	// is due. Zero once sent.
	reopenedAt time.Time
	// revised: the next update should say the cause was revised, once
	// the new cause held since revisedAt for the revise settle.
	revised   bool
	revisedAt time.Time
}

// ScheduleReopenUpdate owes a "failing again" update, due after the
// members settle.
func (p *Pending) ScheduleReopenUpdate(now time.Time) { p.reopenedAt = now }

// ClearReopen drops the owed update: it was sent, or the resolve
// made it moot.
func (p *Pending) ClearReopen() { p.reopenedAt = time.Time{} }

// ReopenOwed reports a "failing again" update not yet sent.
func (p *Pending) ReopenOwed() bool { return !p.reopenedAt.IsZero() }

// ReopenDueAt is when the owed update may go out, given the settle.
func (p *Pending) ReopenDueAt(settle time.Duration) time.Time {
	return p.reopenedAt.Add(settle)
}

// DueReopenUpdate reports an owed update whose settle has passed.
func (p *Pending) DueReopenUpdate(now time.Time, settle time.Duration) bool {
	return !now.Before(p.ReopenDueAt(settle))
}

// MarkRevised owes a "cause revised" update, due after the settle.
func (p *Pending) MarkRevised(now time.Time) {
	p.revised, p.revisedAt = true, now
}

// ClearRevised drops the owed update: it is being sent now.
func (p *Pending) ClearRevised() { p.revised = false }

// RevisedOwed reports a "cause revised" update not yet sent.
func (p *Pending) RevisedOwed() bool { return p.revised }

// RevisedDueAt is when the owed revise update may go out.
func (p *Pending) RevisedDueAt(settle time.Duration) time.Time {
	return p.revisedAt.Add(settle)
}

// Paging state has three questions, each with one helper:
//   - p.Tier == Page: it pages right now (used by delivery and compose).
//   - isPage: it is at the page tier, or is a repeated page held at
//     notify (pageHeld). Reminders, reopening and history treat both alike.
//   - reachedPaging: it is, or was, a page. isPage plus "a page message
//     reached the paging providers" (Paged).

// isPage reports an incident at the page tier, or held at notify because
// a page of the same failure just resolved (see repeatsPage).
func isPage(p *Incident) bool {
	return p.Tier == Page || p.Delivery.PageHeld()
}

// reachedPaging reports an incident that is, or was, a page: its alert
// is open at the paging providers, or held at notify after paging. Its
// resolve closes that alert, so it is never dropped or demoted.
func reachedPaging(p *Incident) bool {
	return isPage(p) || p.Delivery.OpenAtPagers()
}
