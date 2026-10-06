package config

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

// validateProviderRequired reports every unconditional required field of the
// provider catalog that is empty after expansion. The catalog is the single
// source: constructors refuse the same settings at startup.
func validateProviderRequired(cfg *Config) []error {
	required := requiredProviderFields()
	names := make([]string, 0, len(cfg.Alert))
	for name := range cfg.Alert {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		canonical := canonicalProviderName(name)
		for _, field := range required[canonical] {
			if providerValuePresent(cfg.Alert[name], field) {
				continue
			}
			errs = append(errs, fmt.Errorf(
				"alert.%s.%s is required and must not be empty",
				name, field,
			))
		}
	}
	return errs
}

// requiredProviderFields groups unconditional required catalog fields by
// provider. Conditional fields are left to the provider constructors.
func requiredProviderFields() map[string][]string {
	result := make(map[string][]string)
	for _, field := range providerCatalog {
		if field.Required && field.Condition == "" {
			result[field.Provider] = append(
				result[field.Provider], field.Field)
		}
	}
	return result
}

func canonicalProviderName(name string) string {
	name = strings.ToLower(name)
	if canonical, ok := providercatalog.Aliases()[name]; ok {
		return canonical
	}
	return name
}

// providerValuePresent follows dotted field paths and treats nil, blank
// strings and empty lists as absent.
func providerValuePresent(
	settings map[string]interface{}, field string,
) bool {
	var current interface{} = settings
	for _, part := range strings.Split(field, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return false
		}
		current = m[part]
	}
	switch v := current.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	case []string:
		return len(v) > 0
	}
	return true
}

// badDatadogSite reports a site the provider cannot use: kwatch sends
// events to https://api.<site>, so the value must not carry a scheme, path,
// query, fragment, "@" (user info) or whitespace. Anything else (host:port,
// a single-label host, underscores) was accepted before and still is.
func badDatadogSite(site string) bool {
	return strings.ContainsAny(site, "/?#@") ||
		strings.IndexFunc(site, unicode.IsSpace) >= 0
}

// validateDatadogSite rejects an alert.datadog.site that carries more
// than a host name, so the mistake shows up in lint and at startup
// instead of the provider silently failing to start.
func validateDatadogSite(cfg *Config) []error {
	var errs []error
	for _, name := range sortedProviderNames(cfg) {
		if canonicalProviderName(name) != "datadog" {
			continue
		}
		site := strings.TrimSpace(stringValue(cfg.Alert[name]["site"]))
		if site == "" || !badDatadogSite(site) {
			continue
		}
		errs = append(errs, fmt.Errorf(
			"alert.%s.site %q must be a host name such as "+
				"datadoghq.com, datadoghq.eu or us3.datadoghq.com "+
				"(no https://, path, query, @ or spaces)", name, site))
	}
	return errs
}
