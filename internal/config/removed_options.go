package config

import (
	"fmt"
	"sort"
)

// removedProviderOptions lists provider options that no longer change
// incident messages. They are ignored, not rejected, for one release so an
// upgrade does not fail on a config that used to load.
var removedProviderOptions = map[string][]string{
	"slack":      {"title", "text"},
	"discord":    {"title", "text"},
	"mattermost": {"title", "text"},
	"opsgenie":   {"title", "text"},
	"matrix":     {"title", "text"},
	"teams":      {"text"},
	"rocketchat": {"text"},
	"googlechat": {"text"},
}

// removedOptionWarnings reports each removed option that is still set, in a
// stable order. Warnings runs once at startup, so each is logged once.
func removedOptionWarnings(cfg *Config) []string {
	var warnings []string
	providers := make([]string, 0, len(removedProviderOptions))
	for name := range removedProviderOptions {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	for _, name := range providers {
		settings, ok := cfg.Alert[name]
		if !ok {
			continue
		}
		for _, option := range removedProviderOptions[name] {
			if _, set := settings[option]; set {
				warnings = append(warnings, fmt.Sprintf(
					"alert.%s.%s is deprecated and ignored: incident "+
						"messages are written by kwatch. Remove it.",
					name, option))
			}
		}
	}
	return warnings
}
