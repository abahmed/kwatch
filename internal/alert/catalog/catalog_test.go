package catalog

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

func TestProviderCatalogMatchesFactories(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderCatalogHasMetadataForEveryCanonicalProvider(t *testing.T) {
	metadata := make(map[string]bool)
	for _, field := range config.ProviderCatalog() {
		metadata[field.Provider] = true
	}

	for _, name := range providercatalog.Names() {
		// incident.io is an intentional alias of incidentio and shares its
		// configuration schema and factory.
		if name == "incident.io" {
			continue
		}
		if !metadata[name] {
			t.Errorf("provider %q has no configuration metadata", name)
		}
	}
	for name := range metadata {
		if !config.IsKnownProvider(name) {
			t.Errorf("metadata has unknown provider %q", name)
		}
	}
}
