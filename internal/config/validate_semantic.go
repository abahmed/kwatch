package config

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Validate validates the config for semantic correctness and returns a list
// of errors suitable for use in LoadConfig.
func Validate(cfg *Config) []error {
	var errs []error
	errs = append(errs, validateApp(cfg.App)...)
	errs = append(errs, validateLicense(cfg.License)...)
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
	errs = append(errs, validateDatadogSite(cfg)...)
	errs = append(errs, validateTemplates(cfg)...)
	errs = append(errs, validateRunbooks(cfg)...)
	errs = append(errs, validateFallbacks(cfg)...)
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
	if cfg.ResyncSeconds < 0 ||
		(cfg.ResyncSeconds > 0 && cfg.ResyncSeconds < minResyncSeconds) {
		errs = append(errs, fmt.Errorf(
			"resyncSeconds must be 0 (off) or at least %d",
			minResyncSeconds))
	}
	if cfg.OldProblemDays < 0 {
		errs = append(errs, fmt.Errorf(
			"oldProblemDays must be 0 (off) or a number of days"))
	}
	if !validLogFormatter(cfg.App.LogFormatter) {
		errs = append(errs, fmt.Errorf(
			"app.logFormatter %q must be \"text\" or \"json\"",
			cfg.App.LogFormatter))
	}
	if !validEnvironment(cfg.App.Environment) {
		errs = append(errs, fmt.Errorf(
			"app.environment %q must be \"production\" or "+
				"\"non-production\"", cfg.App.Environment))
	}
	if cfg.HealthCheck.Enabled && !validPort(cfg.HealthCheck.Port) {
		errs = append(errs, errors.New(
			"healthCheck.port must be between 1 and 65535 when "+
				"healthCheck.enabled is true",
		))
	}
	if cfg.HealthCheck.StatusEndpoint && !cfg.HealthCheck.Enabled {
		errs = append(errs, errors.New(
			"healthCheck.statusEndpoint requires healthCheck.enabled",
		))
	}
	errs = append(errs, validateDashboard(cfg)...)
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

// minResyncSeconds keeps a typo such as 1 from re-listing every object
// every second.
const minResyncSeconds = 30

// validEnvironment accepts the two values app.environment documents.
// An empty value means production.
func validEnvironment(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "production", EnvironmentNonProduction:
		return true
	}
	return false
}

// validLogFormatter accepts the two formats app.logFormatter documents.
// An empty value means the default.
func validLogFormatter(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text", "json":
		return true
	}
	return false
}

// validateDashboard checks the dashboard listener settings.
func validateDashboard(cfg *Config) []error {
	if !cfg.Dashboard.Enabled {
		return nil
	}
	var errs []error
	if !validPort(cfg.Dashboard.Port) {
		errs = append(errs, errors.New(
			"dashboard.port must be between 1 and 65535 when "+
				"dashboard.enabled is true",
		))
	}
	if cfg.HealthCheck.Enabled &&
		cfg.Dashboard.Port == cfg.HealthCheck.Port {
		errs = append(errs, errors.New(
			"dashboard.port must not equal healthCheck.port",
		))
	}
	errs = append(errs, validateDashboardTLS(cfg.Dashboard)...)
	return errs
}

// validateDashboardTLS checks the TLS file pair and the trusted
// proxy list.
func validateDashboardTLS(d Dashboard) []error {
	var errs []error
	if (d.TLSCertFile == "") != (d.TLSKeyFile == "") {
		errs = append(errs, errors.New(
			"dashboard.tlsCertFile and dashboard.tlsKeyFile go together"))
	}
	for _, entry := range d.TrustedProxies {
		if !validProxyEntry(entry) {
			errs = append(errs, fmt.Errorf(
				"dashboard.trustedProxies entry %q is not an IP "+
					"address or CIDR", entry))
		}
	}
	return errs
}

// validProxyEntry reports whether entry is an IP address or a CIDR.
func validProxyEntry(entry string) bool {
	entry = strings.TrimSpace(entry)
	if _, err := netip.ParsePrefix(entry); err == nil {
		return true
	}
	_, err := netip.ParseAddr(entry)
	return err == nil
}
