package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"text/template"
)

// validateTemplates parses every message template, global and per
// provider, the way delivery compiles them. A template that fails to parse
// is skipped at runtime, so without this check lint would print "config OK"
// while the operator's custom message is silently ignored.
func validateTemplates(cfg *Config) []error {
	var errs []error
	for _, reason := range sortedStringKeys(cfg.Templates) {
		errs = appendTemplateError(
			errs, "templates", reason, cfg.Templates[reason])
	}
	for _, name := range sortedProviderNames(cfg) {
		bodies := compileProviderTemplates(cfg.Alert[name])
		for _, reason := range sortedStringKeys(bodies) {
			errs = appendTemplateError(errs,
				"alert."+name+".templates", reason, bodies[reason])
		}
	}
	return errs
}

func appendTemplateError(
	errs []error, where, reason, body string,
) []error {
	_, err := template.New(reason).Option("missingkey=zero").Parse(body)
	if err != nil {
		return append(errs, fmt.Errorf(
			"%s[%q] is not a valid Go template: %w", where, reason, err))
	}
	return errs
}

// validateRunbooks requires every runbook link to be an absolute http or
// https URL, because it is placed in alert messages as a link.
func validateRunbooks(cfg *Config) []error {
	var errs []error
	for _, reason := range sortedStringKeys(cfg.Runbooks) {
		link := strings.TrimSpace(cfg.Runbooks[reason])
		parsed, err := url.Parse(link)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			errs = append(errs, fmt.Errorf(
				"runbooks[%q] must be an absolute http or https URL",
				reason))
		}
	}
	return errs
}

// validateFallbacks reports a fallback that names no configured provider.
// Delivery would only log it and send nothing when the primary fails.
func validateFallbacks(cfg *Config) []error {
	configured := map[string]bool{}
	for name := range cfg.Alert {
		configured[canonicalProviderName(name)] = true
	}
	var errs []error
	for _, name := range sortedProviderNames(cfg) {
		fallback := stringValue(cfg.Alert[name]["fallback"])
		if fallback == "" || configured[canonicalProviderName(fallback)] {
			continue
		}
		errs = append(errs, fmt.Errorf(
			"alert.%s.fallback names %q, which is not a configured provider",
			name, fallback))
	}
	return errs
}

func sortedProviderNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Alert))
	for name := range cfg.Alert {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
