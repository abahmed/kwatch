package crd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func loadMounted(t *testing.T, body string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestOverlayCannotReenablePrivacyOptOuts(t *testing.T) {
	cfg := loadMounted(t, "telemetry:\n  enabled: false\n"+
		"upgrader:\n  disableUpdateCheck: true\n")
	err := ApplyOverlay(cfg, overlaySpec(map[string]interface{}{
		"telemetry": map[string]interface{}{"enabled": true},
		"upgrader":  map[string]interface{}{"disableUpdateCheck": false},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry.Enabled || !cfg.Upgrader.DisableUpdateCheck {
		t.Fatalf("overlay re-enabled an opt-out: telemetry=%v "+
			"disableUpdateCheck=%v", cfg.Telemetry.Enabled,
			cfg.Upgrader.DisableUpdateCheck)
	}
}

func TestOverlayCanTurnPrivacyChoicesOff(t *testing.T) {
	cfg := loadMounted(t, "{}\n")
	err := ApplyOverlay(cfg, overlaySpec(map[string]interface{}{
		"telemetry": map[string]interface{}{"enabled": false},
		"upgrader":  map[string]interface{}{"disableUpdateCheck": true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry.Enabled || !cfg.Upgrader.DisableUpdateCheck {
		t.Fatal("an overlay may tighten the privacy choices")
	}
}

func probeSpec(http, tcp string) map[string]interface{} {
	return map[string]interface{}{"activeProbeMonitor": map[string]interface{}{
		"enabled": true,
		"http": []interface{}{map[string]interface{}{
			"name": "a", "url": http}},
		"tcp": []interface{}{map[string]interface{}{
			"name": "b", "address": tcp}},
	}}
}

func TestOverlayRefusesSensitiveProbeTargets(t *testing.T) {
	for name, spec := range map[string]map[string]interface{}{
		"metadata http": probeSpec("http://169.254.169.254/latest", "x.test:80"),
		"loopback http": probeSpec("http://127.0.0.1:8080", "x.test:80"),
		"localhost":     probeSpec("http://localhost/", "x.test:80"),
		"metadata tcp":  probeSpec("https://x.test", "169.254.169.254:80"),
		"ipv6 loopback": probeSpec("https://x.test", "[::1]:80"),
		"ipv6 http":     probeSpec("http://[::1]:8080/", "x.test:80"),
		"link-local":    probeSpec("http://[fe80::1]/", "x.test:80"),
		"unspecified":   probeSpec("http://0.0.0.0/", "x.test:80"),
	} {
		t.Run(name, func(t *testing.T) {
			err := ApplyOverlay(loadMounted(t, "{}\n"), overlaySpec(spec))
			if !errorsIs(err, ErrInvalidOverlay) {
				t.Fatalf("err = %v, want ErrInvalidOverlay", err)
			}
		})
	}
}

// In-cluster Services and pods have private addresses and are the main
// thing a probe watches.
func TestOverlayAllowsClusterProbeTargets(t *testing.T) {
	spec := probeSpec("http://10.96.0.10:8080/webhook", "192.168.1.5:5432")
	if err := ApplyOverlay(loadMounted(t, "{}\n"), overlaySpec(spec)); err != nil {
		t.Fatalf("private targets were refused: %v", err)
	}
}

func TestOverlayAllowsPublicAndNamedProbeTargets(t *testing.T) {
	spec := probeSpec("https://svc.apps.svc.cluster.local/healthz",
		"db.example.com:5432")
	if err := ApplyOverlay(loadMounted(t, "{}\n"), overlaySpec(spec)); err != nil {
		t.Fatalf("named targets were refused: %v", err)
	}
}

func TestOverlayKeepsPrivateTargetsTheMountedConfigListed(t *testing.T) {
	cfg := loadMounted(t, `activeProbeMonitor:
  enabled: true
  http:
    - name: a
      url: http://10.1.2.3/health
  tcp:
    - name: b
      address: 10.1.2.4:5432
`)
	if err := ApplyOverlay(cfg, overlaySpec(probeSpec(
		"http://10.1.2.3/health", "10.1.2.4:5432"))); err != nil {
		t.Fatalf("a mounted private target was refused: %v", err)
	}
}

// A name can lead anywhere, so the guard lets it pass and marks it: the
// prober refuses the connection when the name resolves to a loopback,
// link-local or metadata address. Targets the mounted configuration lists
// stay unmarked.
func TestOverlayMarksTheProbeTargetsItAdded(t *testing.T) {
	cfg := loadMounted(t, `activeProbeMonitor:
  enabled: true
  http:
    - name: a
      url: https://mounted.example.com/health
  tcp:
    - name: b
      address: mounted.example.com:5432
`)
	spec := map[string]interface{}{"activeProbeMonitor": map[string]interface{}{
		"enabled": true,
		"http": []interface{}{
			map[string]interface{}{
				"name": "a", "url": "https://mounted.example.com/health"},
			map[string]interface{}{
				"name": "c", "url": "https://added.example.com/"}},
		"tcp": []interface{}{
			map[string]interface{}{
				"name": "b", "address": "mounted.example.com:5432"},
			map[string]interface{}{
				"name": "d", "address": "added.example.com:5432"}},
	}}

	if err := ApplyOverlay(cfg, overlaySpec(spec)); err != nil {
		t.Fatal(err)
	}

	http, tcp := cfg.ActiveProbeMonitor.HTTP, cfg.ActiveProbeMonitor.TCP
	if len(http) != 2 || len(tcp) != 2 {
		t.Fatalf("targets = %d http, %d tcp", len(http), len(tcp))
	}
	if http[0].FromOverlay || tcp[0].FromOverlay {
		t.Error("a mounted target must stay trusted")
	}
	if !http[1].FromOverlay || !tcp[1].FromOverlay {
		t.Error("an overlay target must be marked")
	}
}
