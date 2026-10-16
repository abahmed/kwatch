package config

import (
	"strings"
	"testing"
)

func TestCommunityProviderLoadsIsKnownAndValid(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  communityProvider: slack\n")
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"webhook": "https://example.test/x"},
	}

	if cfg.App.CommunityProvider != "slack" {
		t.Fatalf("communityProvider = %q", cfg.App.CommunityProvider)
	}
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "communityProvider") {
			t.Fatalf("a configured provider was warned about: %s", warning)
		}
	}
	if errs := ValidateConfig(cfg); len(errs) != 0 {
		t.Fatalf("validation errors: %v", errs)
	}
}

func TestCommunityProviderTypoIsReportedAsUnknown(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  communityProvidr: slack\n")

	var found bool
	for _, warning := range Warnings(cfg) {
		found = found || strings.Contains(warning, "app.communityProvidr)")
	}
	if !found || cfg.App.CommunityProvider != "" {
		t.Fatalf("typo handling: %v", Warnings(cfg))
	}
}

func TestCommunityProviderThatIsNotConfiguredWarns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.CommunityProvider = "pagerduty"
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"webhook": "https://example.test/x"},
	}

	var found bool
	for _, warning := range Warnings(cfg) {
		found = found || (strings.Contains(warning, "communityProvider") &&
			strings.Contains(warning, "pagerduty"))
	}
	if !found {
		t.Fatalf("missing warning in %v", Warnings(cfg))
	}
	if errs := ValidateConfig(cfg); len(errs) != 0 {
		t.Fatalf("an unknown name falls back, it must not fail: %v", errs)
	}
}

func TestCommunityProviderRejectsSpaces(t *testing.T) {
	cfg := DefaultConfig()
	cfg.App.CommunityProvider = "slack prod"
	if errs := ValidateConfig(cfg); len(errs) == 0 {
		t.Fatal("a name with a space must be rejected")
	}
}
