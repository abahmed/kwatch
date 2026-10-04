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

// Defaults for Config.
const (
	DefaultSettle         = 75 * time.Second
	DefaultPageSettle     = 15 * time.Second
	DefaultBurstIncidents = 3
	// DefaultReviseSettle covers the failures that usually follow a
	// revised cause within seconds, such as evictions after pressure.
	DefaultReviseSettle = 30 * time.Second
	DefaultHold         = 3 * time.Minute
	DefaultMaxHold      = 30 * time.Minute
	DefaultFlapWindow   = 30 * time.Minute
	DefaultFlapCycles   = 3
	// DefaultRemember keeps resolved incidents for a week, long enough to
	// learn daily routines and to say "3rd time this week".
	DefaultRemember = 7 * 24 * time.Hour
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
