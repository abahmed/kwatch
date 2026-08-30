package config

import (
	"errors"
	"fmt"
	"strings"
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
	for name, values := range map[string][2]float64{
		"filesystem": {cfg.NodeResourceMonitor.FilesystemWarningPercent, cfg.NodeResourceMonitor.FilesystemCriticalPercent},
		"inode":      {cfg.NodeResourceMonitor.InodeWarningPercent, cfg.NodeResourceMonitor.InodeCriticalPercent},
	} {
		warning, critical := values[0], values[1]
		if warning == 0 && critical == 0 {
			continue
		}
		if warning <= 0 || warning > 100 || critical < warning || critical > 100 {
			errs = append(errs, errors.New("nodeResourceMonitor."+name+" thresholds must be 0/disabled or warning <= critical <= 100"))
		}
	}
	return errs
}

func validateRuntimeMetricsMonitor(cfg *Config) []error {
	if !cfg.RuntimeMetricsMonitor.Enabled {
		return nil
	}
	m := cfg.RuntimeMetricsMonitor
	var errs []error
	if m.IntervalSeconds <= 0 {
		errs = append(errs, errors.New("runtimeMetricsMonitor.intervalSeconds must be > 0"))
	}
	if m.MemoryWarningPercent <= 0 || m.MemoryWarningPercent > 100 {
		errs = append(errs, errors.New("runtimeMetricsMonitor.memoryWarningPercent must be in 1..100"))
	}
	if m.MemoryCriticalPercent < m.MemoryWarningPercent || m.MemoryCriticalPercent > 100 {
		errs = append(errs, errors.New("runtimeMetricsMonitor.memoryCriticalPercent must be >= warning and <= 100"))
	}
	if m.CPUWarningPercent <= 0 || m.CPUWarningPercent > 100 {
		errs = append(errs, errors.New("runtimeMetricsMonitor.cpuWarningPercent must be in 1..100"))
	}
	if m.CPUCriticalPercent < m.CPUWarningPercent || m.CPUCriticalPercent > 100 {
		errs = append(errs, errors.New("runtimeMetricsMonitor.cpuCriticalPercent must be >= warning and <= 100"))
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
		if target.Name == "" || target.URL == "" {
			errs = append(errs, errors.New("activeProbeMonitor.http targets require name and url"))
		}
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

func validateKubeletTelemetryMonitor(cfg *Config) []error {
	if !cfg.KubeletTelemetryMonitor.Enabled {
		return nil
	}
	m := cfg.KubeletTelemetryMonitor
	var errs []error
	if m.IntervalSeconds <= 0 {
		errs = append(errs, errors.New("kubeletTelemetryMonitor.intervalSeconds must be > 0"))
	}
	if m.FailureThreshold <= 0 || m.RecoveryThreshold <= 0 {
		errs = append(errs, errors.New("kubeletTelemetryMonitor failure and recovery thresholds must be > 0"))
	}
	for name, values := range map[string][2]float64{
		"memory":           {m.MemoryWarningPercent, m.MemoryCriticalPercent},
		"ephemeralStorage": {m.EphemeralStorageWarningPercent, m.EphemeralStorageCriticalPercent},
		"cpu":              {m.CPUWarningPercent, m.CPUCriticalPercent},
		"cpuThrottling":    {m.CPUThrottlingWarningPercent, m.CPUThrottlingCriticalPercent},
		"psi":              {m.PSIWarningPercent, m.PSICriticalPercent},
		"networkErrorRate": {m.NetworkErrorRateWarning, m.NetworkErrorRateCritical},
		"runtimeErrorRate": {m.RuntimeErrorRateWarning, m.RuntimeErrorRateCritical},
	} {
		if values[0] <= 0 || values[1] < values[0] {
			errs = append(errs, errors.New("kubeletTelemetryMonitor."+name+" thresholds must be positive and warning <= critical"))
		}
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
	for _, rule := range cfg.CrdConfig.FailureConditions {
		parts := strings.Split(rule, "=")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			errs = append(errs, errors.New("crd.failureConditions entries must use ConditionType=Status"))
		}
	}
	for _, rule := range cfg.CrdConfig.GraphReferences {
		parts := strings.SplitN(rule, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			errs = append(errs, errors.New("crd.graphReferences entries must use path=kind"))
		}
	}
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
	errs = append(errs, validateRuntimeMetricsMonitor(cfg)...)
	errs = append(errs, validateActiveProbeMonitor(cfg)...)
	errs = append(errs, validateKubeletTelemetryMonitor(cfg)...)
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
