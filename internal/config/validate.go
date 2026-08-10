package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/model"
)

// InvalidSeverityKeys returns the keys of m whose values are not recognized
// severity levels (critical/high/medium/warning/normal, case-insensitive),
// sorted for stable output. Shared by config.yaml validation and the CRD
// watcher so a typo'd severity is rejected instead of silently ranking normal.
func InvalidSeverityKeys(m map[string]string) []string {
	var keys []string
	for k, v := range m {
		if !model.IsValidSeverity(v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func severityValueError(mapName, key, value string) string {
	return fmt.Sprintf("%s[%q] has invalid severity %q (expected one of critical, high, medium, warning, normal)", mapName, key, value)
}

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

	errs = append(errs, validatePvcWatch(cfg)...)

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

	if cfg.PendingPodMonitor.Enabled && cfg.PendingPodMonitor.Threshold <= 0 {
		errs = append(errs, "pendingPodMonitor.threshold must be > 0")
	}

	if cfg.Workers < 1 {
		errs = append(errs, "workers must be >= 1")
	}

	if cfg.AuditLog.Enabled && cfg.AuditLog.Output == "" {
		errs = append(errs, "auditLog.output must be \"stdout\" or a valid file path when auditLog.enabled is true")
	}

	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Sprintf("unknown alert provider %q", name))
	}

	errs = append(errs, validateRetryJitter(cfg)...)

	for _, k := range InvalidSeverityKeys(cfg.SeverityByReason) {
		errs = append(errs, severityValueError("severityByReason", k, cfg.SeverityByReason[k]))
	}
	for _, k := range InvalidSeverityKeys(cfg.SeverityByOwnerKind) {
		errs = append(errs, severityValueError("severityByOwnerKind", k, cfg.SeverityByOwnerKind[k]))
	}

	return errs
}

// validateRetryJitter checks that every configured retry.jitterFactor is in [0,1].
func validateRetryJitter(cfg *Config) []string {
	var errs []string
	for name, p := range cfg.Alert {
		if r, ok := p["retry"]; ok {
			if rm, ok := r.(map[string]interface{}); ok {
				if jf, ok := rm["jitterFactor"]; ok {
					f, _ := jf.(float64)
					if f < 0 || f > 1 {
						errs = append(errs, fmt.Sprintf("alert.%s.retry.jitterFactor must be between 0 and 1", name))
					}
				}
			}
		}
	}
	return errs
}

// pvcWatchConfigErrors validates the PvcMonitor watch thresholds.
func pvcConfigErrors(m *PvcMonitor) []string {
	var errs []string
	if m.Interval <= 0 {
		errs = append(errs, "pvcMonitor.interval must be > 0")
	}
	if m.Threshold < 0 || m.Threshold > 100 {
		errs = append(errs, "pvcMonitor.threshold must be between 0 and 100")
	}
	if m.CriticalThreshold < 0 || m.CriticalThreshold > 100 {
		errs = append(errs, "pvcMonitor.criticalThreshold must be between 0 and 100")
	}
	if m.CriticalThreshold > 0 && m.Threshold > 0 && m.CriticalThreshold < m.Threshold {
		errs = append(errs, "pvcMonitor.criticalThreshold should be >= threshold")
	}
	if m.ClearThreshold < 0 || m.ClearThreshold > m.Threshold {
		errs = append(errs, "pvcMonitor.clearThreshold must be between 0 and threshold")
	}
	return errs
}

func validatePvcWatch(cfg *Config) []string {
	if !cfg.PvcMonitor.Enabled {
		return nil
	}
	return pvcConfigErrors(&cfg.PvcMonitor)
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
	errs = append(errs, validateCorrelation(cfg)...)
	errs = append(errs, validateMonitors(cfg)...)
	errs = append(errs, validatePvc(cfg)...)
	if cfg.PendingPodMonitor.Enabled && cfg.PendingPodMonitor.Threshold <= 0 {
		errs = append(errs, errors.New("pendingPodMonitor.threshold must be > 0"))
	}
	if cfg.AuditLog.Enabled && cfg.AuditLog.Output == "" {
		errs = append(errs, errors.New("auditLog.output must be \"stdout\" or a valid file path when auditLog.enabled is true"))
	}
	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Errorf("unknown alert provider %q", name))
	}
	for _, k := range InvalidSeverityKeys(cfg.SeverityByReason) {
		errs = append(errs, fmt.Errorf("%s", severityValueError("severityByReason", k, cfg.SeverityByReason[k])))
	}
	for _, k := range InvalidSeverityKeys(cfg.SeverityByOwnerKind) {
		errs = append(errs, fmt.Errorf("%s", severityValueError("severityByOwnerKind", k, cfg.SeverityByOwnerKind[k])))
	}
	return errs
}

// validateCorrelation checks the shared correlation tuning knobs.
func validateCorrelation(cfg *Config) []error {
	var errs []error
	if cfg.Correlation.Window <= 0 {
		errs = append(errs, errors.New("correlation.window must be > 0"))
	}
	if cfg.Correlation.LifecycleInterval <= 0 {
		errs = append(errs, errors.New("correlation.lifecycleInterval must be > 0"))
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
	const maxBaselineEntries = 20000
	if cfg.Correlation.MaxBaseline > maxBaselineEntries {
		errs = append(errs, fmt.Errorf("correlation.maxBaseline=%d may exceed the ~1MB ConfigMap limit (max ~%d)", cfg.Correlation.MaxBaseline, maxBaselineEntries))
	}
	return errs
}

// validateMonitors checks the per-monitor sustain/interval defaults.
func validateMonitors(cfg *Config) []error {
	var errs []error
	if cfg.HeartbeatMonitor.Enabled && cfg.HeartbeatMonitor.Interval <= 0 {
		errs = append(errs, errors.New("heartbeatMonitor.interval must be > 0 when heartbeatMonitor.enabled is true"))
	}
	if cfg.HeartbeatMonitor.Enabled && cfg.HeartbeatMonitor.URL == "" {
		errs = append(errs, errors.New("heartbeatMonitor.url must not be empty when heartbeatMonitor.enabled is true"))
	}
	if cfg.SmartGrouping.WindowSeconds < 0 {
		errs = append(errs, errors.New("smartGrouping.windowSeconds must be >= 0"))
	}
	if cfg.NodeResourceMonitor.Enabled && cfg.NodeResourceMonitor.IntervalSeconds <= 0 {
		errs = append(errs, errors.New("nodeResourceMonitor.intervalSeconds must be > 0"))
	}
	if cfg.NodeResourceMonitor.Enabled {
		if cfg.NodeResourceMonitor.CpuWarning <= 0 {
			errs = append(errs, errors.New("nodeResourceMonitor.cpuWarning must be > 0"))
		}
		if cfg.NodeResourceMonitor.CpuCritical < cfg.NodeResourceMonitor.CpuWarning {
			errs = append(errs, errors.New("nodeResourceMonitor.cpuCritical must be >= cpuWarning"))
		}
		if cfg.NodeResourceMonitor.MemWarning <= 0 {
			errs = append(errs, errors.New("nodeResourceMonitor.memWarning must be > 0"))
		}
		if cfg.NodeResourceMonitor.MemCritical < cfg.NodeResourceMonitor.MemWarning {
			errs = append(errs, errors.New("nodeResourceMonitor.memCritical must be >= memWarning"))
		}
	}
	if cfg.OomMonitor.Enabled {
		if cfg.OomMonitor.Threshold <= 0 {
			errs = append(errs, errors.New("oomMonitor.threshold must be > 0"))
		}
		if cfg.OomMonitor.WindowMinutes <= 0 {
			errs = append(errs, errors.New("oomMonitor.windowMinutes must be > 0"))
		}
	}
	if cfg.TlsMonitor.Enabled && cfg.TlsMonitor.CriticalThreshold < 0 {
		errs = append(errs, errors.New("tlsMonitor.criticalThreshold must be >= 0"))
	}
	if cfg.TlsMonitor.Enabled && cfg.TlsMonitor.Threshold > 0 && cfg.TlsMonitor.CriticalThreshold > cfg.TlsMonitor.Threshold {
		errs = append(errs, errors.New("tlsMonitor.criticalThreshold must be <= threshold"))
	}
	if cfg.TlsMonitor.Enabled && cfg.TlsMonitor.Threshold < 0 {
		errs = append(errs, errors.New("tlsMonitor.threshold must be >= 0"))
	}
	errs = append(errs, validateSustainedMinutes(cfg, "daemonSetMonitor", cfg.DaemonSetMonitor.Enabled, cfg.DaemonSetMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "statefulSetMonitor", cfg.StatefulSetMonitor.Enabled, cfg.StatefulSetMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "pdbMonitor", cfg.PdbMonitor.Enabled, cfg.PdbMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "cronJobMonitor", cfg.CronJobMonitor.Enabled, cfg.CronJobMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "hpaMonitor", cfg.HpaMonitor.Enabled, cfg.HpaMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "rolloutMonitor", cfg.RolloutMonitor.Enabled, cfg.RolloutMonitor.SustainedMinutes)...)
	errs = append(errs, validateSustainedMinutes(cfg, "nodeMonitor", cfg.NodeMonitor.Enabled, cfg.NodeMonitor.SustainedMinutes)...)
	return errs
}

// validateSustainedMinutes checks a monitor's sustain threshold when enabled.
func validateSustainedMinutes(_ *Config, name string, enabled bool, minutes int) []error {
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
	if cfg.PvcMonitor.Threshold <= 0 || cfg.PvcMonitor.Threshold > cfg.PvcMonitor.CriticalThreshold {
		errs = append(errs, errors.New("pvcMonitor requires 0 < threshold <= criticalThreshold"))
	}
	if cfg.PvcMonitor.CriticalThreshold > 100 {
		errs = append(errs, errors.New("pvcMonitor.criticalThreshold must be <= 100"))
	}
	if cfg.PvcMonitor.ClearThreshold < 0 || cfg.PvcMonitor.ClearThreshold > cfg.PvcMonitor.Threshold {
		errs = append(errs, errors.New("pvcMonitor.clearThreshold must be between 0 and threshold"))
	}
	return errs
}
