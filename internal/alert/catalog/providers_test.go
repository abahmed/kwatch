package catalog

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProviderCatalogMatchesConfiguration(t *testing.T) {
	for _, name := range config.KnownProviderNames() {
		if _, ok := factories[name]; !ok {
			t.Errorf("known provider %q has no catalog factory", name)
		}
	}
	for name := range factories {
		if !config.IsKnownProvider(name) {
			t.Errorf("catalog has unknown provider %q", name)
		}
	}
}
