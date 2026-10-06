package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Config tunes the lifecycle. Zero values take the defaults.
type Config struct {
	// Settle is how long a new incident collects findings before its first
	// message. Page-tier incidents use PageSettle.
	Settle     time.Duration
	PageSettle time.Duration
	// BurstIncidents is how many incidents settling at once make a
	// burst. A burst takes Settle, not PageSettle, even at page tier: a
	// shared cause (a node pool, a registry) usually surfaces within it,
	// and one incident then replaces many.
	BurstIncidents int
	// ReviseSettle is how long a revised cause must hold before the
	// "cause revised" update is sent.
	ReviseSettle time.Duration
	// Hold is the base time a root must stay healthy before resolve; it
	// doubles for each recent recovery, up to MaxHold.
	Hold    time.Duration
	MaxHold time.Duration
	// FlapCycles recoveries within FlapWindow make an incident flapping.
	FlapWindow time.Duration
	FlapCycles int
	// Remember is how long a resolved incident is kept for recurrence.
	Remember time.Duration
	// SeverityByReason and SeverityByOwnerKind override the derived tier.
	// Keys match case-insensitively; reasons win over owner kinds.
	SeverityByReason    map[string]string
	SeverityByOwnerKind map[string]string
	// Verifiable reports whether kwatch can currently observe a kind.
	// An incident whose root kind cannot be observed (missing permission,
	// API not served) is never resolved: the absence of findings there
	// is missing data, not recovery. Nil treats every kind as verifiable.
	Verifiable func(inventory.Kind) bool
	// IDNonce is the nonce new incident IDs carry, 4 hex characters.
	// Empty draws a random one. Restore adopts the nonce of the newest
	// restored ID, so the nonce lives as long as the store's incidents:
	// a reset or a new volume starts with a new nonce, and its IDs never
	// equal earlier ones. Tests set it for deterministic IDs.
	IDNonce string
}

// Defaults for Config counts; the durations are in timings.go.
const (
	DefaultBurstIncidents = 3
	DefaultFlapCycles     = 3
)

func (c Config) withDefaults() Config {
	set := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	set(&c.Settle, DefaultSettle)
	set(&c.PageSettle, DefaultPageSettle)
	set(&c.ReviseSettle, DefaultReviseSettle)
	if c.BurstIncidents <= 0 {
		c.BurstIncidents = DefaultBurstIncidents
	}
	set(&c.Hold, DefaultHold)
	set(&c.MaxHold, DefaultMaxHold)
	set(&c.FlapWindow, DefaultFlapWindow)
	set(&c.Remember, DefaultRemember)
	if c.FlapCycles <= 0 {
		c.FlapCycles = DefaultFlapCycles
	}
	return c
}

// hold returns the resolve hold for an incident with n recent recoveries.
func (c Config) hold(recentCycles int) time.Duration {
	hold := c.Hold
	for i := 0; i < recentCycles && hold < c.MaxHold; i++ {
		hold *= 2
	}
	return min(hold, c.MaxHold)
}

// holdFor is how long p must stay healthy before it resolves: the base
// hold doubled for each recent recovery, and multiplied by ChronicFactor
// when p is a chronic flapper, never beyond MaxHold.
func (m *Manager) holdFor(p *Incident, now time.Time) time.Duration {
	hold := m.cfg.hold(len(recent(p.Cycles, now, m.cfg.FlapWindow)))
	if len(recent(p.Occurrences, now, ChronicWindow)) >= ChronicOccurrences {
		hold = min(hold*ChronicFactor, m.cfg.MaxHold)
	}
	return hold
}
