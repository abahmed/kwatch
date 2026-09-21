package catalog

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery/transport"
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

func TestProviderFactoriesConstructWithEmptyRuntimeValues(t *testing.T) {
	for _, name := range ProviderNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("factory panicked: %v", recovered)
				}
			}()
			provider := NewProvider(name, map[string]interface{}{},
				transport.ProviderContext{})
			if provider == nil {
				t.Fatal("factory returned nil provider")
			}
		})
	}
}
