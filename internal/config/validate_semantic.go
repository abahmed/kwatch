package config

import (
	"errors"
	"fmt"
)

// Validate validates the config for semantic correctness and returns a list
// of errors suitable for use in LoadConfig.
func Validate(cfg *Config) []error {
	var errs []error
	errs = append(errs, validateCorrelation(cfg)...)
	errs = append(errs, validateMonitors(cfg)...)
	errs = append(errs, validatePvc(cfg)...)
	if cfg.PendingPodMonitor.Enabled && cfg.PendingPodMonitor.Threshold <= 0 {
		errs = append(
			errs,
			errors.New("pendingPodMonitor.threshold must be > 0"),
		)
	}
	if cfg.AuditLog.Enabled && cfg.AuditLog.Output == "" {
		errs = append(
			errs,
			errors.New(
				"auditLog.output must be \"stdout\" or a valid file path when "+
					"auditLog.enabled is true",
			),
		)
	}
	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Errorf("unknown alert provider %q", name))
	}
	for _, k := range InvalidSeverityKeys(cfg.SeverityByReason) {
		errs = append(
			errs,
			fmt.Errorf(
				"%s",
				severityValueError(
					"severityByReason",
					k,
					cfg.SeverityByReason[k],
				),
			),
		)
	}
	for _, k := range InvalidSeverityKeys(cfg.SeverityByOwnerKind) {
		errs = append(
			errs,
			fmt.Errorf(
				"%s",
				severityValueError(
					"severityByOwnerKind",
					k,
					cfg.SeverityByOwnerKind[k],
				),
			),
		)
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
		errs = append(
			errs,
			errors.New("correlation.lifecycleInterval must be > 0"),
		)
	}
	if cfg.Correlation.Escalation.Enabled {
		for i, t := range cfg.Correlation.Escalation.Tiers {
			if t <= 0 {
				errs = append(
					errs,
					fmt.Errorf("escalation.tiers[%d] must be > 0", i),
				)
			}
			if i > 0 && t <= cfg.Correlation.Escalation.Tiers[i-1] {
				errs = append(
					errs,
					fmt.Errorf(
						"escalation.tiers must be strictly ascending "+
							"(tiers[%d]=%d <= tiers[%d]=%d)",
						i,
						t,
						i-1,
						cfg.Correlation.Escalation.Tiers[i-1],
					),
				)
			}
		}
	}
	if cfg.Correlation.ResolveHoldDown < 0 {
		errs = append(
			errs,
			errors.New("correlation.resolveHoldDown must be >= 0"),
		)
	}
	if cfg.Correlation.ResolveHoldDown > cfg.Correlation.Window*60 {
		errs = append(
			errs,
			errors.New(
				"correlation.resolveHoldDown must be <= correlation.window "+
					"(in seconds)",
			),
		)
	}
	if cfg.Correlation.MaxBaseline < 0 {
		errs = append(errs, errors.New("correlation.maxBaseline must be >= 0"))
	}
	const maxBaselineEntries = 20000
	if cfg.Correlation.MaxBaseline > maxBaselineEntries {
		errs = append(
			errs,
			fmt.Errorf(
				"correlation.maxBaseline=%d may exceed the ~1MB ConfigMap "+
					"limit (max ~%d)",
				cfg.Correlation.MaxBaseline,
				maxBaselineEntries,
			),
		)
	}
	return errs
}
