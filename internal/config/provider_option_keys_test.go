package config

import (
	"strings"
	"testing"
)

func TestWarningsFlagMisspelledProviderOption(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"webhok": "x", "channel": "#ops"},
	}

	warnings := Warnings(cfg)

	want := `alert.slack.webhok is not a slack option and is ignored; ` +
		`did you mean "webhook"?`
	if !containsString(warnings, want) {
		t.Fatalf("warnings = %v, want %q", warnings, want)
	}
}

func TestWarningsAcceptCatalogAndSharedOptions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"webhook": {
			"url":       "https://example.test",
			"basicAuth": map[string]interface{}{"username": "u"},
			"retry":     map[string]interface{}{"maxAttempts": 2},
			"routes":    []interface{}{}, "fallback": "slack",
			"templates": map[string]interface{}{},
		},
		"slack": {"title": "removed option has its own warning"},
	}

	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "is not a") {
			t.Fatalf("unexpected unknown-option warning: %s", warning)
		}
	}
}

func TestWarningsOmitSuggestionWithoutCloseMatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"discord": {"completelyDifferent": true},
	}

	warnings := unknownProviderOptionWarnings(cfg)

	if len(warnings) != 1 || strings.Contains(warnings[0], "did you mean") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestLintWarnsWhenNoProviderIsConfigured(t *testing.T) {
	cfg := DefaultConfig()

	if errs := ValidateConfig(cfg); len(errs) > 0 {
		t.Fatalf("no provider must not be a lint error: %v", errs)
	}
	if !containsString(LintWarnings(cfg), "no alert providers configured") {
		t.Fatal("lint must warn when no provider is configured")
	}
	if containsString(Warnings(cfg), "no alert providers configured") {
		t.Fatal("startup warnings must not include the lint-only finding")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
