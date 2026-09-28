package problem

import "time"

// Config tunes the lifecycle. Zero values take the defaults.
type Config struct {
	// Settle is how long a new problem collects signals before its first
	// message. Page-tier problems use PageSettle.
	Settle     time.Duration
	PageSettle time.Duration
	// Hold is the base time a root must stay healthy before resolve; it
	// doubles for each recent recovery, up to MaxHold.
	Hold    time.Duration
	MaxHold time.Duration
	// FlapCycles recoveries within FlapWindow make a problem flapping.
	FlapWindow time.Duration
	FlapCycles int
	// Remember is how long a resolved problem is kept for recurrence.
	Remember time.Duration
}

// Defaults for Config.
const (
	DefaultSettle     = 75 * time.Second
	DefaultPageSettle = 15 * time.Second
	DefaultHold       = 3 * time.Minute
	DefaultMaxHold    = 30 * time.Minute
	DefaultFlapWindow = 30 * time.Minute
	DefaultFlapCycles = 3
	DefaultRemember   = 24 * time.Hour
)

func (c Config) withDefaults() Config {
	set := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	set(&c.Settle, DefaultSettle)
	set(&c.PageSettle, DefaultPageSettle)
	set(&c.Hold, DefaultHold)
	set(&c.MaxHold, DefaultMaxHold)
	set(&c.FlapWindow, DefaultFlapWindow)
	set(&c.Remember, DefaultRemember)
	if c.FlapCycles <= 0 {
		c.FlapCycles = DefaultFlapCycles
	}
	return c
}

// hold returns the resolve hold for a problem with n recent recoveries.
func (c Config) hold(recentCycles int) time.Duration {
	hold := c.Hold
	for i := 0; i < recentCycles && hold < c.MaxHold; i++ {
		hold *= 2
	}
	return min(hold, c.MaxHold)
}
