package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

func routedRoutes() []interface{} {
	return []interface{}{map[string]interface{}{
		"namespaces": []interface{}{"shop"},
	}}
}

// deliveryConfig has three providers with routing, a fallback, an hourly
// budget, a provider template and a global template.
func deliveryConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"webhook": {"url": "https://hooks.example.test/w"},
		"slack": {
			"token": "xoxb-test", "channel": "#ops",
			"routes":       routedRoutes(),
			"fallback":     "webhook",
			"hourlyBudget": 5,
			"templates":    map[string]interface{}{"oomkilled": "{{.Text}}"},
		},
		"discord": {"webhook": "https://discord.example.test/x"},
	}
	cfg.Templates = map[string]string{"error": "{{.Text}}"}
	cfg.Runtime = config.CompileRuntimeConfig(cfg)
	return cfg
}

func TestDefaultAlertSetupSetsNoDeliveryPro(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"webhook": "https://hooks.example.test/x"},
	}
	assert.Empty(t, configuredPro(cfg))
	assert.Empty(t, inactivePro(feature.CommunityEntitlements(), cfg))
}

func TestConfiguredProListsDeliveryFeatures(t *testing.T) {
	got := configuredPro(deliveryConfig())
	assert.Contains(t, got, feature.ProviderRouting)
	assert.Contains(t, got, feature.NativeThreads)
	assert.Contains(t, got, feature.CustomTemplates)
	assert.Contains(t, got, extraProviders)
}

func TestCommunityNamesInactiveDeliveryFeatures(t *testing.T) {
	got := inactivePro(feature.CommunityEntitlements(), deliveryConfig())
	assert.Contains(t, got,
		"alert providers beyond 1 (2 inactive: slack, webhook)")
	assert.Contains(t, got, "delivery.routing")
	assert.Contains(t, got, "delivery.threads")
	assert.Contains(t, got, "delivery.templates")
	assert.NotContains(t, got, string(extraProviders))
	assert.Empty(t, inactivePro(feature.ProEntitlements(), deliveryConfig()))
}

func TestCommunityRunsOneProviderChosenByName(t *testing.T) {
	cfg := deliveryConfig()
	got := restrictConfig(cfg, feature.CommunityEntitlements())
	providers := config.RuntimeConfigFor(got).Delivery().Providers()

	require.Len(t, providers, 1)
	assert.Equal(t, "discord", providers[0].Name, "first by sorted name")
	assert.Len(t, cfg.Alert, 3, "the original is untouched")
	for i := 0; i < 20; i++ {
		again := config.RuntimeConfigFor(
			restrictConfig(cfg, feature.CommunityEntitlements()))
		assert.Equal(t, "discord", again.Delivery().Providers()[0].Name)
	}
}

func TestCommunityIgnoresRoutesFallbackAndBudget(t *testing.T) {
	cfg := deliveryConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": cfg.Alert["slack"],
	}
	got := restrictConfig(cfg, feature.CommunityEntitlements())
	provider := config.RuntimeConfigFor(got).Delivery().Providers()[0]

	assert.Empty(t, provider.Routes, "every alert goes to the provider")
	assert.Empty(t, provider.FallbackName)
	assert.Equal(t, config.DefaultHourlyBudget, provider.HourlyBudget,
		"the operator budget is ignored; the default stays")
	assert.Equal(t, "xoxb-test", provider.Settings["token"],
		"the provider itself still works")
	assert.NotEmpty(t, cfg.Alert["slack"]["routes"], "original untouched")
}

func TestCommunityIgnoresOperatorTemplates(t *testing.T) {
	got := restrictConfig(deliveryConfig(), feature.CommunityEntitlements())
	rt := config.RuntimeConfigFor(got)

	assert.Empty(t, rt.Delivery().Templates())
	for _, provider := range rt.Delivery().Providers() {
		assert.Empty(t, provider.Templates, provider.Name)
	}
}

func TestProKeepsEveryProviderRouteAndTemplate(t *testing.T) {
	got := restrictConfig(deliveryConfig(), feature.ProEntitlements())
	rt := config.RuntimeConfigFor(got)

	require.Len(t, rt.Delivery().Providers(), 3)
	assert.NotEmpty(t, rt.Delivery().Templates())
	var slack config.ProviderRuntime
	for _, p := range rt.Delivery().Providers() {
		if p.Name == "slack" {
			slack = p
		}
	}
	assert.NotEmpty(t, slack.Routes)
	assert.Equal(t, "webhook", slack.FallbackName)
	assert.Equal(t, 5, slack.HourlyBudget)
	assert.NotEmpty(t, slack.Templates)
}

func TestNilEntitlementsRestrictDeliveryLikeCommunity(t *testing.T) {
	got := restrictConfig(deliveryConfig(), nil)
	providers := config.RuntimeConfigFor(got).Delivery().Providers()
	require.Len(t, providers, 1)
	assert.Empty(t, providers[0].Routes)
}

func TestRemovedProviderDoesNotTakeTheCommunitySlot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"line":  {"token": "gone"},
		"slack": {"webhook": "https://hooks.example.test/x"},
	}
	got := restrictConfig(cfg, feature.CommunityEntitlements())
	providers := config.RuntimeConfigFor(got).Delivery().Providers()
	require.Len(t, providers, 1)
	assert.Equal(t, "slack", providers[0].Name)
	assert.Empty(t, inactivePro(feature.CommunityEntitlements(), cfg))
}
