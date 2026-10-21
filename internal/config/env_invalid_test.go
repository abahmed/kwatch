package config

import (
	"strings"
	"testing"
)

func TestLoadConfigIgnoresInvalidEnvironmentBooleanWithWarning(t *testing.T) {
	for _, name := range []string{
		watchSecretsEnv, crdEnabledEnv, telemetryEnv, clusterCheckinsEnv,
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
	t.Setenv(clusterCheckinsEnv, "nope")
	t.Setenv(crdEnabledEnv, "maybe")
	cfg := loadConfigText(t, "crd:\n  enabled: true\n")

	if cfg.Runtime.Application().WatchSecrets {
		t.Error("an invalid KWATCH_WATCH_SECRETS must not watch Secrets")
	}
	if cfg.Telemetry.Enabled {
		t.Error("an invalid KWATCH_TELEMETRY must turn telemetry off")
	}
	if cfg.ClusterCheckins {
		t.Error("an invalid KWATCH_CLUSTER_CHECKINS must turn check-ins off")
	}
	if !cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Error("an invalid KWATCH_CRD_ENABLED must keep the file's value")
	}
}

func TestClusterCheckinsEnvironmentCanOnlyOptOut(t *testing.T) {
	t.Setenv(clusterCheckinsEnv, "false")
	cfg := loadConfigText(t, "clusterCheckins: true\n")
	if cfg.ClusterCheckins {
		t.Fatal("KWATCH_CLUSTER_CHECKINS=false must win over the file")
	}
	t.Setenv(clusterCheckinsEnv, "true")
	cfg = loadConfigText(t, "clusterCheckins: false\n")
	if cfg.ClusterCheckins {
		t.Fatal("KWATCH_CLUSTER_CHECKINS=true must not turn check-ins on")
	}
}

func TestRebuildAfterOverlayWarnsOnInvalidEnvironmentOnce(t *testing.T) {
	t.Setenv(crdEnabledEnv, "maybe")
	cfg := DefaultConfig()
	if err := RebuildAfterOverlay(cfg); err != nil {
		t.Fatalf("an invalid boolean must not fail the rebuild: %v", err)
	}
	if err := RebuildAfterOverlay(cfg); err != nil {
		t.Fatal(err)
	}
	if n := len(cfg.envWarnings); n != 1 {
		t.Fatalf("env warnings after two rebuilds = %d, want 1", n)
	}
}
