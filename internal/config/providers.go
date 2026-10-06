package config

import (
	"strings"

	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

// IsKnownProvider reports whether name is a supported provider key.
func IsKnownProvider(name string) bool {
	name = strings.ToLower(name)
	for _, known := range providercatalog.Names() {
		if name == known {
			return true
		}
	}
	return false
}

// removedProviders are provider keys whose upstream service is gone. A
// config that still has one loads, the section is ignored, and Warnings
// says so, so an upgrade does not fail on a file that used to work.
var removedProviders = map[string]bool{"line": true}

func isRemovedProvider(name string) bool {
	return removedProviders[strings.ToLower(name)]
}

// KnownProviderNames returns a stable, sorted copy for catalogs and tooling.
func KnownProviderNames() []string {
	return providercatalog.Names()
}
