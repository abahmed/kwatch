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

// KnownProviderNames returns a stable, sorted copy for catalogs and tooling.
func KnownProviderNames() []string {
	return providercatalog.Names()
}
