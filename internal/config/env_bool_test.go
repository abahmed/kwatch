package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseEnvBool(t *testing.T) {
	tests := []struct {
		in         string
		value, set bool
		bad        bool
	}{
		{"", false, false, false}, {"  ", false, false, false},
		{"true", true, true, false}, {"TRUE", true, true, false},
		{"1", true, true, false}, {"on", true, true, false},
		{"Yes", true, true, false}, {"false", false, true, false},
		{"0", false, true, false}, {"OFF", false, true, false},
		{" no ", false, true, false}, {"flase", false, false, true},
		{"2", false, false, true}, {"enabled", false, false, true},
	}
	for _, tc := range tests {
		value, set, err := ParseEnvBool("X", tc.in)
		if (err != nil) != tc.bad || value != tc.value || set != tc.set {
			t.Errorf("%q = (%v, %v, %v)", tc.in, value, set, err)
		}
	}
}

func TestInvalidEnvironmentBooleanFailsLoad(t *testing.T) {
	for _, name := range []string{
		watchSecretsEnv, crdEnabledEnv, telemetryEnv,
	} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + "/config.yaml"
			if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CONFIG_FILE", path)
			t.Setenv(name, "flase")
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("want an error naming %s, got %v", name, err)
			}
		})
	}
}

func TestEnvironmentAcceptsOnOffYesNo(t *testing.T) {
	t.Setenv(watchSecretsEnv, "off")
	t.Setenv(crdEnabledEnv, "yes")
	cfg := loadConfigText(t, "watch:\n  secrets: true\n")
	if cfg.Runtime.Application().WatchSecrets {
		t.Fatal("KWATCH_WATCH_SECRETS=off must turn Secrets off")
	}
	if !cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Fatal("KWATCH_CRD_ENABLED=yes must enable the overlay")
	}
}

func TestRebuildAfterOverlayRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv(crdEnabledEnv, "maybe")
	if err := RebuildAfterOverlay(DefaultConfig()); err == nil {
		t.Fatal("an invalid boolean must fail the rebuild")
	}
}

func TestResyncSecondsMinimum(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		ok      bool
	}{{0, true}, {30, true}, {300, true}, {1, false}, {29, false},
		{-1, false}} {
		cfg := DefaultConfig()
		cfg.ResyncSeconds = tc.seconds
		bad := false
		for _, err := range Validate(cfg) {
			bad = bad || strings.Contains(err.Error(), "resyncSeconds")
		}
		if bad == tc.ok {
			t.Errorf("resyncSeconds %d: rejected=%v", tc.seconds, bad)
		}
	}
}

func TestLogFormatterMustBeTextOrJSON(t *testing.T) {
	for format, ok := range map[string]bool{
		"": true, "text": true, "json": true, "JSON": true,
		"yaml": false, "jsno": false,
	} {
		cfg := DefaultConfig()
		cfg.App.LogFormatter = format
		bad := false
		for _, err := range Validate(cfg) {
			bad = bad || strings.Contains(err.Error(), "logFormatter")
		}
		if bad == ok {
			t.Errorf("logFormatter %q: rejected=%v", format, bad)
		}
	}
}
