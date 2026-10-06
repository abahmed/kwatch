package config

import (
	"strings"
	"testing"
)

func TestRemovedLineProviderIsIgnoredWithWarning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"line":    {"token": "x"},
		"webhook": {"url": "http://example.test/hook"},
	}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("line section must not fail validation: %v", errs)
	}
	found := false
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "LINE Notify was shut down") &&
			strings.Contains(warning, "ignored") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a LINE removed warning")
	}
	runtime := CompileRuntimeConfig(cfg)
	for _, name := range runtime.Delivery().ProviderNames() {
		if name == "line" {
			t.Fatal("line provider must not be built")
		}
	}
}

func TestWarningsFlagRemovedProviderOptions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"slack": {"title": "x", "webhook": "y"},
		"teams": {"title": "kept"},
	}
	got := Warnings(cfg)
	if len(got) != 1 || !strings.Contains(got[0], "alert.slack.title") {
		t.Fatalf("unexpected warnings: %v", got)
	}
}
