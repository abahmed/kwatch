package announce

import "github.com/abahmed/kwatch/internal/incident"

// A restored incident whose announcement a listing carried (a roll-up or
// the startup summary named it) and that never spoke on its own is still
// owed its introduction after a restart: the introduction a roll-up
// promises waits in memory (pipeline introduce.go), so a restart before
// it went out loses it. After the restored listing at the end of the
// grace, such incidents are handed to the introductions once. The
// listing records the introduction sent (Listing.Followed), so it is
// never sent twice, across restarts too.

// noteRestoredIntros collects the restored incidents still owed their
// introduction, once per start, when the restore grace is over.
func (c *Collector) noteRestoredIntros() {
	if c.introsNoted {
		return
	}
	c.introsNoted = true
	for _, p := range c.env.Incidents.RestoredFailing() {
		if !p.Delivery.RolledUp() || !currentNews(p) ||
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
