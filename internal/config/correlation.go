package config

// Correlation config struct
type Correlation struct {
	// Window is the time window for correlating events. Events outside this
	// window start a new incident. A bare number counts minutes; a duration
	// string ("10m", "1h") is also accepted.
	Window Minutes `yaml:"window"`

	// LifecycleInterval is the interval (in minutes) for checking
	// lifecycle transitions (stale, resolved). Default 1.
	LifecycleInterval int `yaml:"lifecycleInterval"`

	// ResolveHoldDown is the seconds to wait after a condition clears before
	// emitting "resolved". If it recurs within this window the incident stays
	// open (flap dampening). Default 300. Set to 0 to resolve immediately.
	// A bare number counts seconds; "5m" is also accepted.
	ResolveHoldDown Seconds `yaml:"resolveHoldDown"`

	// Escalation configures restart-count-based severity escalation.
	Escalation EscalationConfig `yaml:"escalation"`

	// Renotify configures periodic re-notification via
	// intervalBySeverity["default"].
	Renotify RenotifyConfig `yaml:"renotify"`

	// MaxBaseline is the maximum number of baseline entries to keep.
	// Default 5000.
	MaxBaseline int `yaml:"maxBaseline"`

	// CooldownMinutes is accepted and ignored. It used to gate a
	// container-reason pre-filter that dropped repeated crashes before the
	// engine saw them, which defeated the engine's own post-resolve
	// cooldown. That cooldown is Window, and it is now the only one.
	//
	// Deprecated: use correlation.window.
	CooldownMinutes int `yaml:"cooldownMinutes"`
}

// RenotifyConfig configures periodic re-notification for active incidents.
type RenotifyConfig struct {
	// IntervalBySeverity is the minimum time (in minutes) between
	// renotifications,
	// keyed by severity ("normal", "high", "critical"). Use "default" key as
	// fallback when a severity has no entry. 0 disables renotify.
	IntervalBySeverity map[string]int `yaml:"intervalBySeverity"`
	// MaxPerIncident is the maximum number of renotifications per incident.
	// Default 3.
	MaxPerIncident int `yaml:"maxPerIncident"`
}

// EscalationConfig configures severity escalation when restart count
// crosses configured thresholds.
type EscalationConfig struct {
	// Enabled if set to true, severity escalates when restart count
	// crosses configured tier boundaries.
	Enabled bool `yaml:"enabled"`

	// Tiers is an ordered list of restart count thresholds. Crossing the
	// first raises severity to "high", crossing the second to "critical".
	// There is nothing above critical, so further tiers have no effect.
	// Default [3, 10]: at 3+ restarts → "high", 10+ → "critical".
	Tiers []int `yaml:"tiers"`
}
