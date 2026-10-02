package config

import (
	"fmt"
	"sort"
	"strings"
)

// sharedProviderOptions are read by delivery for every provider, so the
// provider catalog does not list them for each one.
var sharedProviderOptions = []string{
	"fallback", "hourlyBudget", "retry", "routes", "templates",
}

// providerOptionKeys maps each catalog provider to the top-level option
// keys it reads. A nested catalog field such as basicAuth.username
// contributes its first segment.
var providerOptionKeys = buildProviderOptionKeys(providerCatalog)

func buildProviderOptionKeys(
	fields []ProviderField,
) map[string]map[string]bool {
	keys := make(map[string]map[string]bool)
	for _, field := range fields {
		if keys[field.Provider] == nil {
			keys[field.Provider] = make(map[string]bool)
			for _, shared := range sharedProviderOptions {
				keys[field.Provider][shared] = true
			}
		}
		top, _, _ := strings.Cut(field.Field, ".")
		keys[field.Provider][top] = true
	}
	for provider, removed := range removedProviderOptions {
		// Removed options have their own deprecation warning.
		for _, option := range removed {
			if keys[provider] != nil {
				keys[provider][option] = true
			}
		}
	}
	return keys
}

// unknownProviderOptionWarnings reports alert.<provider> keys the provider
// does not read, such as alert.slack.webhok, with a suggestion when one
// known key is a near miss. Unknown providers are a validation error
// elsewhere and are skipped here.
func unknownProviderOptionWarnings(cfg *Config) []string {
	providers := make([]string, 0, len(cfg.Alert))
	for name := range cfg.Alert {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	var warnings []string
	for _, provider := range providers {
		known, ok := providerOptionKeys[provider]
		if !ok {
			continue
		}
		for _, key := range sortedKeys(cfg.Alert[provider]) {
			if known[key] {
				continue
			}
			warnings = append(warnings,
				unknownOptionWarning(provider, key, known))
		}
	}
	return warnings
}

func unknownOptionWarning(
	provider, key string, known map[string]bool,
) string {
	text := fmt.Sprintf(
		"alert.%s.%s is not a %s option and is ignored", provider, key,
		provider)
	if suggestion := closestKey(key, known); suggestion != "" {
		text += fmt.Sprintf("; did you mean %q?", suggestion)
	}
	return text
}

// closestKey returns the only known key within two edits of key, ignoring
// case, or "" when there is no single clear candidate.
func closestKey(key string, known map[string]bool) string {
	best, bestDistance, ties := "", 3, 0
	for candidate := range known {
		distance := editDistance(
			strings.ToLower(key), strings.ToLower(candidate))
		switch {
		case distance < bestDistance:
			best, bestDistance, ties = candidate, distance, 0
		case distance == bestDistance:
			ties++
		}
	}
	if ties > 0 {
		return ""
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1,
				previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func sortedKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
