package config

import (
	"errors"
	"fmt"
)

// validateMonitors checks the per-monitor sustain/interval defaults.
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

func validateNodeResourceMonitor(cfg *Config) []error {
	var errs []error
	if !cfg.NodeResourceMonitor.Enabled {
		return errs
	}
	if cfg.NodeResourceMonitor.IntervalSeconds <= 0 {
		errs = append(
			errs,
			errors.New("nodeResourceMonitor.intervalSeconds must be > 0"),
		)
	}
	if cfg.NodeResourceMonitor.CpuWarning <= 0 {
		errs = append(
			errs,
			errors.New("nodeResourceMonitor.cpuWarning must be > 0"),
		)
	}
	if cfg.NodeResourceMonitor.CpuCritical <
		cfg.NodeResourceMonitor.CpuWarning {
		errs = append(
			errs,
			errors.New("nodeResourceMonitor.cpuCritical must be >= cpuWarning"),
		)
	}
	if cfg.NodeResourceMonitor.MemWarning <= 0 {
		errs = append(
			errs,
			errors.New("nodeResourceMonitor.memWarning must be > 0"),
		)
	}
	if cfg.NodeResourceMonitor.MemCritical <
		cfg.NodeResourceMonitor.MemWarning {
		errs = append(
			errs,
			errors.New("nodeResourceMonitor.memCritical must be >= memWarning"),
		)
	}
	return errs
}

func validateOomMonitor(cfg *Config) []error {
	var errs []error
	if !cfg.OomMonitor.Enabled {
		return errs
	}
	if cfg.OomMonitor.Threshold <= 0 {
		errs = append(errs, errors.New("oomMonitor.threshold must be > 0"))
	}
	if cfg.OomMonitor.WindowMinutes <= 0 {
		errs = append(errs, errors.New("oomMonitor.windowMinutes must be > 0"))
	}
	return errs
}

func validateTlsMonitor(cfg *Config) []error {
	var errs []error
	if !cfg.TlsMonitor.Enabled {
		return errs
	}
	if cfg.TlsMonitor.CriticalThreshold < 0 {
		errs = append(
			errs,
			errors.New("tlsMonitor.criticalThreshold must be >= 0"),
		)
	}
	if cfg.TlsMonitor.Threshold > 0 &&
		cfg.TlsMonitor.CriticalThreshold > cfg.TlsMonitor.Threshold {
		errs = append(
			errs,
			errors.New("tlsMonitor.criticalThreshold must be <= threshold"),
		)
	}
	if cfg.TlsMonitor.Threshold < 0 {
		errs = append(errs, errors.New("tlsMonitor.threshold must be >= 0"))
	}
	return errs
}

func validateMonitors(cfg *Config) []error {
	var errs []error
	errs = append(errs, validateHeartbeatMonitor(cfg)...)
	if cfg.SmartGrouping.NamespaceFanOutThreshold < 0 {
		errs = append(
			errs,
			errors.New("smartGrouping.namespaceFanOutThreshold must be >= 0"),
		)
	}
	if cfg.SmartGrouping.WindowSeconds < 0 {
		errs = append(
			errs,
			errors.New("smartGrouping.windowSeconds must be >= 0"),
		)
	}
	errs = append(errs, validateNodeResourceMonitor(cfg)...)
	errs = append(errs, validateOomMonitor(cfg)...)
	errs = append(errs, validateTlsMonitor(cfg)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"daemonSetMonitor",
			cfg.DaemonSetMonitor.Enabled,
			cfg.DaemonSetMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"statefulSetMonitor",
			cfg.StatefulSetMonitor.Enabled,
			cfg.StatefulSetMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"pdbMonitor",
			cfg.PdbMonitor.Enabled,
			cfg.PdbMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"cronJobMonitor",
			cfg.CronJobMonitor.Enabled,
			cfg.CronJobMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"hpaMonitor",
			cfg.HpaMonitor.Enabled,
			cfg.HpaMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"rolloutMonitor",
			cfg.RolloutMonitor.Enabled,
			cfg.RolloutMonitor.SustainedMinutes,
		)...)
	errs = append(
		errs,
		validateSustainedMinutes(
			cfg,
			"nodeMonitor",
			cfg.NodeMonitor.Enabled,
			cfg.NodeMonitor.SustainedMinutes,
		)...)
	return errs
}

// validateSustainedMinutes checks a monitor's sustain threshold when enabled.
func validateSustainedMinutes(
	_ *Config,
	name string,
	enabled bool,
	minutes int,
) []error {
	if enabled && minutes < 0 {
		return []error{fmt.Errorf("%s.sustainedMinutes must be >= 0", name)}
	}
	return nil
}

// validatePvc checks PvcMonitor thresholds when enabled.
func validatePvc(cfg *Config) []error {
	if !cfg.PvcMonitor.Enabled {
		return nil
	}
	var errs []error
	if cfg.PvcMonitor.Interval <= 0 {
		errs = append(errs, errors.New("pvcMonitor.interval must be > 0"))
	}
	if cfg.PvcMonitor.Threshold <= 0 ||
		cfg.PvcMonitor.Threshold > cfg.PvcMonitor.CriticalThreshold {
		errs = append(
			errs,
			errors.New(
				"pvcMonitor requires 0 < threshold <= criticalThreshold",
			),
		)
	}
	if cfg.PvcMonitor.CriticalThreshold > 100 {
		errs = append(
			errs,
			errors.New("pvcMonitor.criticalThreshold must be <= 100"),
		)
	}
	if cfg.PvcMonitor.ClearThreshold < 0 ||
		cfg.PvcMonitor.ClearThreshold > cfg.PvcMonitor.Threshold {
		errs = append(
			errs,
			errors.New(
				"pvcMonitor.clearThreshold must be between 0 and threshold",
			),
		)
	}
	return errs
}
