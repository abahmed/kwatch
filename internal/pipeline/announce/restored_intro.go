package announce

import "github.com/abahmed/kwatch/internal/incident"

// A restored incident whose announcement a listing carried (a roll-up or
// the startup summary named it) and that never spoke on its own is still
// owed its introduction after a restart: the introduction a roll-up
// promises waits in memory (pipeline introduce.go), so a restart before
// it went out loses it. After the restored listing at the end of the
// grace, such incidents are handed to the introductions once. The
// introduction sent is recorded in the startup marker (NoteSpoke), not
// only in the listing that named it: a restored listing is not
// persisted, so without it every restart introduced it again.
//
// The startup marker forgets an incident once it is over, but a
// resolved incident can reopen under the same ID. So the incident
// record keeps the decided introduction too (Introduced): an incident
// is introduced at most once in its whole life. One the restored
// summary only counted as unchanged was not named by it, so it is
// owed nothing (restoredNews).

// noteRestoredIntros collects the restored incidents still owed their
// introduction, once per start, when the restore grace is over.
func (c *Collector) noteRestoredIntros() {
	if c.introsNoted {
		return
	}
	c.introsNoted = true
	if c.pruneSpoke() {
		c.saveSummary()
	}
	for _, p := range c.env.Incidents.RestoredFailing() {
		if !p.Delivery.RolledUp() || p.Introduced() || !currentNews(p) ||
			c.ListedBy(p.ID) == "" || c.SpokeOnItsOwn(p.ID) ||
			(c.env.Scope != nil && !c.env.Scope(p)) {
			continue
		}
		c.restoredIntros = append(c.restoredIntros, p)
	}
}

// TakeRestoredIntros returns, once, the restored incidents still owed
// their introduction.
func (c *Collector) TakeRestoredIntros() []incident.Incident {
	out := c.restoredIntros
	c.restoredIntros = nil
	return out
}
