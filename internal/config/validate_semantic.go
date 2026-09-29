package config

import (
	"errors"
	"fmt"
)

// Validate validates the config for semantic correctness and returns a list
// of errors suitable for use in LoadConfig.
func Validate(cfg *Config) []error {
	var errs []error
	errs = append(errs, validateApp(cfg.App)...)
	errs = append(errs, validateProbes(cfg)...)
	errs = append(errs, validateSelectors(cfg)...)
	errs = append(errs, validatePodNamePatterns(cfg)...)
	errs = append(errs, validateAlertRetries(cfg)...)
	if cfg.ResyncSeconds < 0 {
		errs = append(errs, errors.New("resyncSeconds must be >= 0"))
	}
	if cfg.HealthCheck.Enabled && cfg.HealthCheck.Port <= 0 {
		errs = append(errs, errors.New(
			"healthCheck.port must be > 0 when healthCheck.enabled is true",
		))
	}
	if cfg.HealthCheck.Enabled &&
		(cfg.HealthCheck.Diagnostics || cfg.HealthCheck.Pprof) &&
		cfg.HealthCheck.DiagnosticsToken == "" {
		errs = append(errs, errors.New(
			"healthCheck.diagnosticsToken must be set when diagnostics or "+
				"pprof is enabled",
		))
	}
	for _, text := range validateMaintenance(cfg) {
		errs = append(errs, errors.New(text))
	}
	for _, text := range validateRetryJitter(cfg) {
		errs = append(errs, errors.New(text))
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
