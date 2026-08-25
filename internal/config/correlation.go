package config

// Correlation config struct
type Correlation struct {
	// Window is the time window (in minutes) for correlating events.
	// Events outside this window start a new incident.
	Window int `yaml:"window"`

	// LifecycleInterval is the interval (in minutes) for checking
	// lifecycle transitions (stale, resolved). Default 1.
	LifecycleInterval int `yaml:"lifecycleInterval"`

	// ResolveHoldDown is the seconds to wait after a condition clears before
	// emitting "resolved". If it recurs within this window the incident stays
	// open (flap dampening). Default 0 = resolve immediately.
	ResolveHoldDown int `yaml:"resolveHoldDown"`

	// Escalation configures restart-count-based severity escalation.
	Escalation EscalationConfig `yaml:"escalation"`

	// Renotify configures periodic re-notification via intervalBySeverity["default"].
	Renotify RenotifyConfig `yaml:"renotify"`

	// MaxBaseline is the maximum number of baseline entries to keep.
	// Default 5000.
	MaxBaseline int `yaml:"maxBaseline"`

	// CooldownMinutes is the minimum time (in minutes) between re-alerts
	// for the same container crash reason. When a container crashes with
	// the same reason/message/exit code, subsequent alerts are suppressed
	// until this cooldown expires. Default 10. Set to 0 to disable the
	// cooldown (always re-alert on identical crashes).
	CooldownMinutes int `yaml:"cooldownMinutes"`
}

// RenotifyConfig configures periodic re-notification for active incidents.
type RenotifyConfig struct {
	// IntervalBySeverity is the minimum time (in minutes) between renotifications,
	// keyed by severity ("normal", "high", "critical"). Use "default" key as
	// fallback when a severity has no entry. 0 disables renotify.
	IntervalBySeverity map[string]int `yaml:"intervalBySeverity"`
	// MaxPerIncident is the maximum number of renotifications per incident. Default 3.
	MaxPerIncident int `yaml:"maxPerIncident"`
}

// EscalationConfig configures severity escalation when restart count
// crosses configured thresholds.
type EscalationConfig struct {
	// Enabled if set to true, severity escalates when restart count
	// crosses configured tier boundaries.
	Enabled bool `yaml:"enabled"`

	// Tiers is an ordered list of restart count thresholds. When the
	// RestartCount crosses a tier, severity escalates one level.
	// Example: [3, 10, 50] → at 3+ restarts → "high", 10+ → "critical".
	Tiers []int `yaml:"tiers"`
}
