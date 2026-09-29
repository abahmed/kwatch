package config

import (
	"errors"
)

// validateHeartbeatMonitor rejects a negative ping interval.
func validateHeartbeatMonitor(cfg *Config) []error {
	var errs []error
	if cfg.HeartbeatMonitor.Enabled && cfg.HeartbeatMonitor.Interval < 0 {
		errs = append(
			errs,
			errors.New(
				"heartbeatMonitor.interval must be >= 0 when "+
					"heartbeatMonitor.enabled is true",
			),
		)
	}
	if cfg.HeartbeatMonitor.Enabled && cfg.HeartbeatMonitor.URL == "" {
		errs = append(
			errs,
			errors.New(
				"heartbeatMonitor.url must not be empty when "+
					"heartbeatMonitor.enabled is true",
			),
		)
	}
	return errs
}

func validateActiveProbeMonitor(cfg *Config) []error {
	if !cfg.ActiveProbeMonitor.Enabled {
		return nil
	}
	m := cfg.ActiveProbeMonitor
	var errs []error
	if m.IntervalSeconds <= 0 {
		errs = append(errs, errors.New("activeProbeMonitor.intervalSeconds must be > 0"))
	}
	if m.TimeoutSeconds <= 0 {
		errs = append(errs, errors.New("activeProbeMonitor.timeoutSeconds must be > 0"))
	}
	if m.FailureThreshold <= 0 || m.RecoveryThreshold <= 0 {
		errs = append(errs, errors.New("activeProbeMonitor thresholds must be > 0"))
	}
	for _, target := range m.HTTP {
		errs = append(errs, validateHTTPProbeTarget(target)...)
	}
	for _, target := range m.TCP {
		if target.Name == "" || target.Address == "" {
			errs = append(errs, errors.New("activeProbeMonitor.tcp targets require name and address"))
		}
	}
	for _, target := range m.DNS {
		if target.Name == "" || target.Host == "" {
			errs = append(errs, errors.New("activeProbeMonitor.dns targets require name and host"))
		}
	}
	return errs
}

func validateHTTPProbeTarget(target HTTPProbeTarget) []error {
	var errs []error
	if target.Name == "" || target.URL == "" {
		errs = append(errs, errors.New("activeProbeMonitor.http targets require name and url"))
	}
	if target.ExpectedStatus > 0 && (target.ExpectedStatus < 100 || target.ExpectedStatus > 599) {
		errs = append(errs, errors.New("activeProbeMonitor.http expectedStatus must be between 100 and 599"))
	}
	if target.LatencyWarningMs > 0 && target.LatencyCriticalMs > 0 && target.LatencyCriticalMs < target.LatencyWarningMs {
		errs = append(errs, errors.New("activeProbeMonitor.http latencyCriticalMs must be >= latencyWarningMs"))
	}
	return errs
}

func validateProbes(cfg *Config) []error {
	return append(validateHeartbeatMonitor(cfg),
		validateActiveProbeMonitor(cfg)...)
}
