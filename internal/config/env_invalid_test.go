package config

import (
	"strings"
	"testing"
)

func TestLoadConfigIgnoresInvalidEnvironmentBooleanWithWarning(t *testing.T) {
	for _, name := range []string{
		watchSecretsEnv, crdEnabledEnv, telemetryEnv,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "flase")
			cfg := loadConfigText(t, "{}\n")
			found := false
			for _, w := range Warnings(cfg) {
				found = found || (strings.Contains(w, name) &&
					strings.Contains(w, "flase"))
			}
			if !found {
				t.Fatalf("no warning names %s: %v", name, Warnings(cfg))
			}
		})
	}
}

func TestInvalidEnvironmentBooleanFallsBackToTheSafeSide(t *testing.T) {
	t.Setenv(watchSecretsEnv, "flase")
	t.Setenv(telemetryEnv, "ture")
	t.Setenv(crdEnabledEnv, "maybe")
	cfg := loadConfigText(t, "crd:\n  enabled: true\n")

	if cfg.Runtime.Application().WatchSecrets {
		t.Error("an invalid KWATCH_WATCH_SECRETS must not watch Secrets")
	}
	if cfg.Telemetry.Enabled {
		t.Error("an invalid KWATCH_TELEMETRY must turn telemetry off")
	}
	if !cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Error("an invalid KWATCH_CRD_ENABLED must keep the file's value")
	}
}

func TestRemovedClusterCheckinsLoadWithANotice(t *testing.T) {
	cfg := loadConfigText(t, "clusterCheckins: false\n")
	if !hasNoticeOnce(cfg, removedCheckinsNotice) {
		t.Fatalf("no deprecation notice: %v", Warnings(cfg))
	}
	t.Setenv(clusterCheckinsEnv, "false")
	cfg = loadConfigText(t, "{}\n")
	if !hasNoticeOnce(cfg, removedCheckinsNotice) {
		t.Fatalf("no notice for the environment: %v", Warnings(cfg))
	}
}

func TestNoCheckinsNoticeWhenUnset(t *testing.T) {
	if hasNoticeOnce(loadConfigText(t, "{}\n"), removedCheckinsNotice) {
		t.Fatal("notice without clusterCheckins")
	}
}

func hasNoticeOnce(cfg *Config, want string) bool {
	n := 0
	for _, w := range Warnings(cfg) {
		if w == want {
			n++
		}
	}
	return n == 1
}
