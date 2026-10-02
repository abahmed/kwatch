package config

import "testing"

func TestWatchSecretsDefaultsOn(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  clusterName: dev\n")
	if !cfg.Watch.Secrets || !cfg.Runtime.Application().WatchSecrets {
		t.Fatal("Secrets are watched by default")
	}
}

func TestWatchSecretsCanBeTurnedOff(t *testing.T) {
	cfg := loadConfigText(t, "watch:\n  secrets: false\n")
	if cfg.Runtime.Application().WatchSecrets {
		t.Fatal("watch.secrets: false must reach the runtime snapshot")
	}
}

func TestWatchSecretsEnvironmentOnlyTurnsOff(t *testing.T) {
	t.Setenv(watchSecretsEnv, "false")
	cfg := loadConfigText(t, "watch:\n  secrets: true\n")
	if cfg.Runtime.Application().WatchSecrets {
		t.Fatal("KWATCH_WATCH_SECRETS=false must win")
	}

	t.Setenv(watchSecretsEnv, "true")
	cfg = loadConfigText(t, "watch:\n  secrets: false\n")
	if cfg.Runtime.Application().WatchSecrets {
		t.Fatal("the environment never turns Secrets back on")
	}
}

func TestKubeletInsecureSkipVerifyReachesRuntime(t *testing.T) {
	cfg := loadConfigText(t, "kubelet:\n  insecureSkipVerify: true\n")
	if !cfg.Runtime.Application().KubeletInsecureSkipVerify {
		t.Fatal("kubelet.insecureSkipVerify must reach the runtime")
	}
	cfg = loadConfigText(t, "app:\n  clusterName: dev\n")
	if cfg.Runtime.Application().KubeletInsecureSkipVerify {
		t.Fatal("kubelet certificates are verified by default")
	}
}

func TestCRDEnvironmentOverridesFileInBothDirections(t *testing.T) {
	t.Setenv(crdEnabledEnv, "true")
	cfg := loadConfigText(t, "app:\n  clusterName: dev\n")
	if !cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Fatal("KWATCH_CRD_ENABLED=true must enable the overlay")
	}

	t.Setenv(crdEnabledEnv, "false")
	cfg = loadConfigText(t, "crd:\n  enabled: true\n")
	if cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Fatal("KWATCH_CRD_ENABLED=false must disable the overlay")
	}

	t.Setenv(crdEnabledEnv, "")
	cfg = loadConfigText(t, "crd:\n  enabled: true\n")
	if !cfg.Runtime.Lifecycle().CRDEnabled() {
		t.Fatal("an unset KWATCH_CRD_ENABLED keeps config.yaml's value")
	}
}

func TestRebuildAfterOverlayReappliesEnvironment(t *testing.T) {
	t.Setenv(watchSecretsEnv, "false")
	cfg := DefaultConfig()
	cfg.Watch.Secrets = true

	if err := RebuildAfterOverlay(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.Application().WatchSecrets {
		t.Fatal("an overlay must not turn Secret watching back on")
	}
}
