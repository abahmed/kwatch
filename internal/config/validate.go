package config

import (
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
	return fmt.Sprintf(
		"%s[%q] has invalid severity %q (expected one of critical, high, "+
			"medium, warning, normal)",
		mapName,
		key,
		value,
	)
}

// ValidateConfig renders the semantic validation for `kwatch lint`.
//
// It used to be a second, independently written validator: it checked
// maxRecentLogLines, workers, healthCheck.port and the PVC hysteresis that
// Validate did not, while Validate checked the escalation tier ordering, the
// resolveHoldDown-versus-window relationship and the baseline ceiling that it
// did not. So `kwatch lint` could pass a config the process would refuse to
// start on, and vice versa -- the one thing a config linter must never do.
// One validator, two renderings. Warnings are deliberately not included:
// they must not decide lint's exit status, so the command prints them
// separately.
func ValidateConfig(cfg *Config) []string {
	errs := make([]string, 0)
	for _, err := range Validate(cfg) {
		errs = append(errs, err.Error())
	}
	if len(cfg.Alert) == 0 {
		// Only lint reports this: a running kwatch with no provider is a
		// legitimate configuration (it still exposes /incidents and metrics),
		// but it is almost never what someone linting a file intended.
		errs = append(errs, "no alert providers configured")
	}
	return errs
}

func validateMaintenance(cfg *Config) []string {
	if cfg.Maintenance.Enabled && strings.TrimSpace(cfg.Maintenance.Annotation) == "" {
		return []string{"maintenance.annotation must not be empty when maintenance is enabled"}
	}
	return nil
}

// validateRetryJitter checks that every configured retry.jitterFactor is in
// [0,1].
func validateRetryJitter(cfg *Config) []string {
	var errs []string
	for name, p := range cfg.Alert {
		if r, ok := p["retry"]; ok {
			if rm, ok := r.(map[string]interface{}); ok {
				if jf, ok := rm["jitterFactor"]; ok {
					f, _ := jf.(float64)
					if f < 0 || f > 1 {
						errs = append(
							errs,
							fmt.Sprintf(
								"alert.%s.retry.jitterFactor must be between "+
									"0 and 1",
								name,
							),
						)
					}
				}
			}
		}
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
