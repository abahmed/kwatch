package config

import (
	"fmt"
	"net/url"
	"strings"
)

// validateServiceNow checks what the ServiceNow provider would refuse at
// startup: an instance address that is not a bare https host, and
// credentials that are not exactly one complete pair (user and password,
// or clientID and clientSecret). The messages never quote a value.
func validateServiceNow(cfg *Config) []error {
	var errs []error
	for _, name := range sortedProviderNames(cfg) {
		if canonicalProviderName(name) != "servicenow" {
			continue
		}
		settings := cfg.Alert[name]
		if err := serviceNowInstanceError(settings); err != "" {
			errs = append(errs, fmt.Errorf("alert.%s.instanceURL %s",
				name, err))
		}
		if err := serviceNowAuthError(settings); err != "" {
			errs = append(errs, fmt.Errorf("alert.%s: %s", name, err))
		}
	}
	return errs
}

func serviceNowInstanceError(settings map[string]interface{}) string {
	raw := strings.TrimSpace(stringValue(settings["instanceURL"]))
	if raw == "" {
		// The required-field check reports it.
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return "must be an https address such as " +
			"https://example.service-now.com"
	}
	if parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" {
		return "must be the instance address only, without a path, " +
			"query or user information"
	}
	return ""
}

func serviceNowAuthError(settings map[string]interface{}) string {
	present := func(key string) bool {
		return providerValuePresent(settings, key)
	}
	basic := present("user") || present("password")
	oauth := present("clientID") || present("clientSecret")
	switch {
	case basic && oauth:
		return "set either user and password, or clientID and " +
			"clientSecret, not both"
	case basic && !(present("user") && present("password")):
		return "basic authentication needs both user and password"
	case oauth && !(present("clientID") && present("clientSecret")):
		return "OAuth needs both clientID and clientSecret"
	case !basic && !oauth:
		return "set user and password, or clientID and clientSecret"
	}
	return ""
}
