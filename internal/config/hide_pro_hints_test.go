package config

import (
	"strings"
	"testing"
)

func TestHideProHintsDefaultsToFalse(t *testing.T) {
	if DefaultConfig().App.HideProHints {
		t.Fatal("the Pro hint is shown unless the operator hides it")
	}
	if CompileRuntimeConfig(DefaultConfig()).Application().HideProHints {
		t.Fatal("the runtime snapshot must default to showing the hint")
	}
}

func TestHideProHintsLoadsIsKnownAndValid(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  hideProHints: true\n")

	if !cfg.App.HideProHints ||
		!cfg.Runtime.Application().HideProHints {
		t.Fatal("app.hideProHints must reach the runtime snapshot")
	}
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "hideProHints") {
			t.Fatalf("a known key was reported: %s", warning)
		}
	}
	if errs := ValidateConfig(cfg); len(errs) != 0 {
		t.Fatalf("validation errors: %v", errs)
	}
}

func TestHideProHintsTypoIsReportedAsUnknown(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  hideProHint: true\n")

	var found bool
	for _, warning := range Warnings(cfg) {
		found = found || strings.Contains(warning, "app.hideProHint)")
	}
	if !found || cfg.App.HideProHints {
		t.Fatalf("typo handling: %v", Warnings(cfg))
	}
}
