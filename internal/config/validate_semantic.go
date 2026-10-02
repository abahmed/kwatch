package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Validate validates the config for semantic correctness and returns a list
// of errors suitable for use in LoadConfig.
func Validate(cfg *Config) []error {
	var errs []error
	errs = append(errs, validateApp(cfg.App)...)
	errs = append(errs, validateProbes(cfg)...)
	errs = append(errs, validateSelectors(cfg)...)
	errs = append(errs, validatePodNamePatterns(cfg)...)
	errs = append(errs, validateSilenceRules(cfg)...)
	errs = append(errs, validateEmptyMatchers(cfg)...)
	errs = append(errs, validateAlertRetries(cfg)...)
	errs = append(errs, validateAlertRoutes(cfg)...)
	errs = append(errs, validateLifecycleSettings(cfg)...)
	for _, text := range validateMaintenance(cfg) {
		errs = append(errs, errors.New(text))
	}
	for _, text := range validateRetryJitter(cfg) {
		errs = append(errs, errors.New(text))
	}
	for _, name := range unknownProviders(cfg) {
		errs = append(errs, fmt.Errorf("unknown alert provider %q", name))
	}
	errs = append(errs, validateProviderRequired(cfg)...)
	errs = append(errs, validateSeverityMaps(cfg)...)
	errs = append(errs, caseCollisionErrors(
		"severityByReason", cfg.SeverityByReason)...)
	errs = append(errs, caseCollisionErrors(
		"severityByOwnerKind", cfg.SeverityByOwnerKind)...)
	return errs
}

// validateLifecycleSettings checks resync, health check and audit log.
func validateLifecycleSettings(cfg *Config) []error {
	var errs []error
	if cfg.ResyncSeconds < 0 {
		errs = append(errs, errors.New("resyncSeconds must be >= 0"))
	}
	if cfg.HealthCheck.Enabled && !validPort(cfg.HealthCheck.Port) {
		errs = append(errs, errors.New(
			"healthCheck.port must be between 1 and 65535 when "+
				"healthCheck.enabled is true",
		))
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
	return errs
}

// validateSeverityMaps checks the severity override values.
func validateSeverityMaps(cfg *Config) []error {
	var errs []error
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

// caseCollisionErrors reports keys of m that differ only in case. Severity
// overrides match case-insensitively, so such keys would be ambiguous.
func caseCollisionErrors(mapName string, m map[string]string) []error {
	groups := make(map[string][]string)
	for key := range m {
		lower := strings.ToLower(strings.TrimSpace(key))
		groups[lower] = append(groups[lower], key)
	}
	lowers := make([]string, 0, len(groups))
	for lower, keys := range groups {
		if len(keys) > 1 {
			lowers = append(lowers, lower)
		}
	}
	sort.Strings(lowers)
	var errs []error
	for _, lower := range lowers {
		keys := groups[lower]
		sort.Strings(keys)
		errs = append(errs, fmt.Errorf(
			"%s keys %q differ only in case", mapName, keys))
	}
	return errs
}
