package alert

import (
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProviderFactoriesMatchKnownProviders(t *testing.T) {
	for provider := range config.KnownProviders {
		if _, ok := providerFactories[provider]; !ok {
			t.Errorf("known provider %q has no factory", provider)
		}
	}
	for provider := range providerFactories {
		if !config.KnownProviders[provider] {
			t.Errorf("factory registered for unknown provider %q", provider)
		}
	}
}
