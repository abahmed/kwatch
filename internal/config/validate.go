package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ValidateConfig checks the config for common misconfiguration issues and
// returns a list of human-readable problems.
func ValidateConfig(cfg *Config) []string {
	var errs []string

	if len(cfg.Alert) == 0 {
		errs = append(errs, "no alert providers configured")
	}

	if cfg.HealthCheck.Enabled && cfg.HealthCheck.Port <= 0 {
		errs = append(errs, "healthCheck.port must be > 0 when healthCheck.enabled is true")
	}

	if cfg.MaxRecentLogLines < 0 {
		errs = append(errs, "maxRecentLogLines must be >= 0")
	}

	if cfg.PvcMonitor.Enabled {
		if cfg.PvcMonitor.Interval <= 0 {
			errs = append(errs, "pvcMonitor.interval must be > 0")
		}
		if cfg.PvcMonitor.Threshold < 0 || cfg.PvcMonitor.Threshold > 100 {
			errs = append(errs, "pvcMonitor.threshold must be between 0 and 100")
		}
		if cfg.PvcMonitor.CriticalThreshold < 0 || cfg.PvcMonitor.CriticalThreshold > 100 {
			errs = append(errs, "pvcMonitor.criticalThreshold must be between 0 and 100")
		}
		if cfg.PvcMonitor.CriticalThreshold > 0 && cfg.PvcMonitor.Threshold > 0 &&
			cfg.PvcMonitor.CriticalThreshold < cfg.PvcMonitor.Threshold {
			errs = append(errs, "pvcMonitor.criticalThreshold should be >= threshold")
		}
		if cfg.PvcMonitor.ClearThreshold < 0 {
			errs = append(errs, "pvcMonitor.clearThreshold must be >= 0")
		} else if cfg.PvcMonitor.ClearThreshold > cfg.PvcMonitor.Threshold {
			cfg.PvcMonitor.ClearThreshold = cfg.PvcMonitor.Threshold
		}
	}

	if cfg.Correlation.Window <= 0 {
		errs = append(errs, "correlation.window must be > 0")
	}
	if cfg.Correlation.LifecycleInterval <= 0 {
		errs = append(errs, "correlation.lifecycleInterval must be > 0")
	}
	if cfg.Correlation.MaxBaseline < 0 {
		errs = append(errs, "correlation.maxBaseline must be >= 0")
	}

	if cfg.Correlation.Escalation.Enabled {
		for i, t := range cfg.Correlation.Escalation.Tiers {
			if t <= 0 {
				errs = append(errs, fmt.Sprintf("correlation.escalation.tiers[%d] must be > 0", i))
			}
		}
	}

	if cfg.StormConfig.Enabled {
		if cfg.StormConfig.Threshold <= 0 {
			errs = append(errs, "stormConfig.threshold must be > 0")
		}
		if cfg.StormConfig.WindowMinutes <= 0 {
			errs = append(errs, "stormConfig.windowMinutes must be > 0")
		}
		if cfg.StormConfig.DigestIntervalMinutes <= 0 {
			errs = append(errs, "stormConfig.digestIntervalMinutes must be > 0")
		}
	}

	if cfg.PendingPodMonitor.Enabled && cfg.PendingPodMonitor.Threshold <= 0 {
		errs = append(errs, "pendingPodMonitor.threshold must be > 0")
	}

	if cfg.Workers < 1 {
		errs = append(errs, "workers must be >= 1")
	}

	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Sprintf("unknown alert provider %q", name))
	}

	return errs
}

func unknownProviders(cfg *Config) []string {
	var unknown []string
	for name := range cfg.Alert {
		if !KnownProviders[strings.ToLower(name)] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// Validate validates the config for semantic correctness and returns a list
// of errors suitable for use in LoadConfig.
func Validate(cfg *Config) []error {
	var errs []error
	if cfg.StormConfig.Enabled {
		if cfg.StormConfig.Threshold <= 0 {
			errs = append(errs, errors.New("storm.threshold must be > 0"))
		}
		if cfg.StormConfig.WindowMinutes <= 0 {
			errs = append(errs, errors.New("storm.windowMinutes must be > 0"))
		}
		if cfg.StormConfig.DigestIntervalMinutes <= 0 {
			errs = append(errs, errors.New("storm.digestIntervalMinutes must be > 0"))
		}
	}
	if cfg.Correlation.Window <= 0 {
		errs = append(errs, errors.New("correlation.window must be > 0"))
	}
	if cfg.Correlation.LifecycleInterval <= 0 {
		errs = append(errs, errors.New("correlation.lifecycleInterval must be > 0"))
	}
	if cfg.HeartbeatMonitor.Enabled && cfg.HeartbeatMonitor.Interval < 0 {
		errs = append(errs, errors.New("heartbeatMonitor.interval must be >= 0"))
	}
	if cfg.TlsMonitor.Enabled && cfg.TlsMonitor.Threshold < 0 {
		errs = append(errs, errors.New("tlsMonitor.threshold must be >= 0"))
	}
	if cfg.Correlation.Escalation.Enabled {
		for i, t := range cfg.Correlation.Escalation.Tiers {
			if t <= 0 {
				errs = append(errs, fmt.Errorf("escalation.tiers[%d] must be > 0", i))
			}
			if i > 0 && t <= cfg.Correlation.Escalation.Tiers[i-1] {
				errs = append(errs, fmt.Errorf("escalation.tiers must be strictly ascending (tiers[%d]=%d <= tiers[%d]=%d)", i, t, i-1, cfg.Correlation.Escalation.Tiers[i-1]))
			}
		}
	}
	if cfg.Correlation.ResolveHoldDown < 0 {
		errs = append(errs, errors.New("correlation.resolveHoldDown must be >= 0"))
	}
	if cfg.Correlation.ResolveHoldDown > cfg.Correlation.Window*60 {
		errs = append(errs, errors.New("correlation.resolveHoldDown must be <= correlation.window (in seconds)"))
	}
	if cfg.Correlation.MaxBaseline < 0 {
		errs = append(errs, errors.New("correlation.maxBaseline must be >= 0"))
	}
	const maxBaselineEntries = 3600
	if cfg.Correlation.MaxBaseline > maxBaselineEntries {
		errs = append(errs, fmt.Errorf("correlation.maxBaseline=%d risks exceeding the ~1MB ConfigMap limit (max ~%d, or gzip)", cfg.Correlation.MaxBaseline, maxBaselineEntries))
	}
	if cfg.PendingPodMonitor.Enabled && cfg.PendingPodMonitor.Threshold <= 0 {
		errs = append(errs, errors.New("pendingPodMonitor.threshold must be > 0"))
	}
	if cfg.PvcMonitor.Enabled {
		if cfg.PvcMonitor.Interval <= 0 {
			errs = append(errs, errors.New("pvcMonitor.interval must be > 0"))
		}
		if cfg.PvcMonitor.Threshold <= 0 || cfg.PvcMonitor.Threshold > cfg.PvcMonitor.CriticalThreshold {
			errs = append(errs, errors.New("pvcMonitor requires 0 < threshold <= criticalThreshold"))
		}
		if cfg.PvcMonitor.CriticalThreshold > 100 {
			errs = append(errs, errors.New("pvcMonitor.criticalThreshold must be <= 100"))
		}
		if cfg.PvcMonitor.ClearThreshold < 0 || cfg.PvcMonitor.ClearThreshold > cfg.PvcMonitor.Threshold {
			cfg.PvcMonitor.ClearThreshold = cfg.PvcMonitor.Threshold
		}
	}
	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Errorf("unknown alert provider %q", name))
	}
	return errs
}
