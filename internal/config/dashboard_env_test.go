package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestDashboardEnvironmentOverridesTheFile(t *testing.T) {
	t.Setenv(dashboardEnabledEnv, "true")
	t.Setenv(dashboardPortEnv, "9090")
	t.Setenv(dashboardProxiesEnv, "10.0.0.0/8, 192.168.1.5 ,")
	cfg := loadConfigText(t, "dashboard:\n  enabled: false\n  port: 8061\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	want := []string{"10.0.0.0/8", "192.168.1.5"}
	if !d.Enabled || d.Port != 9090 ||
		!reflect.DeepEqual(d.TrustedProxies, want) {
		t.Fatalf("dashboard = %+v, want the environment's values", d)
	}

	t.Setenv(dashboardEnabledEnv, "")
	t.Setenv(dashboardPortEnv, "")
	t.Setenv(dashboardProxiesEnv, "")
	cfg = loadConfigText(t, "dashboard:\n  enabled: true\n"+
		"  trustedProxies: [10.1.0.0/16]\n")
	d = cfg.Runtime.Lifecycle().Dashboard()
	if len(d.TrustedProxies) != 0 {
		t.Fatalf("a set but empty environment list clears the file: %+v", d)
	}
}

func TestDashboardEnvironmentKeepsTheFileWhenUnset(t *testing.T) {
	cfg := loadConfigText(t, "dashboard:\n  enabled: true\n"+
		"  trustedProxies: [10.1.0.0/16]\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if !d.Enabled || d.Port != 8061 ||
		!reflect.DeepEqual(d.TrustedProxies, []string{"10.1.0.0/16"}) {
		t.Fatalf("unset environment must keep the file: %+v", d)
	}
}

func TestDashboardEnvironmentIgnoresBadPortWithAWarning(t *testing.T) {
	t.Setenv(dashboardPortEnv, "eighty")
	cfg := loadConfigText(t, "dashboard:\n  enabled: true\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if d.Port != 8061 {
		t.Fatalf("bad environment changed the settings: %+v", d)
	}
	if got := strings.Join(cfg.envWarnings, "\n"); !strings.Contains(got,
		dashboardPortEnv) {
		t.Fatalf("warnings = %q", got)
	}
}

func TestDashboardValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dashboard.Enabled = true
	cfg.Dashboard.TrustedProxies = []string{"10.0.0.0/8", "::1", "bogus"}
	if !hasDashboardError(Validate(cfg)) {
		t.Fatal("a trusted proxy that is not an address passed")
	}
	cfg.Dashboard.TrustedProxies = []string{"10.0.0.0/8", "::1"}
	if hasDashboardError(Validate(cfg)) {
		t.Fatal("valid trusted proxies were rejected")
	}
	cfg.Dashboard.TLSCertFile = "/tls/tls.crt"
	if !hasDashboardError(Validate(cfg)) {
		t.Fatal("a certificate without a key passed")
	}
	cfg.Dashboard.TLSKeyFile = "/tls/tls.key"
	if hasDashboardError(Validate(cfg)) {
		t.Fatal("a certificate and key were rejected")
	}
}

func hasDashboardError(errs []error) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), "dashboard") {
			return true
		}
	}
	return false
}

func TestDashboardEnvironmentSetsTLSFiles(t *testing.T) {
	t.Setenv(dashboardCertEnv, "/tls/tls.crt")
	t.Setenv(dashboardKeyEnv, "/tls/tls.key")
	cfg := loadConfigText(t, "dashboard:\n  enabled: true\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if d.TLSCertFile != "/tls/tls.crt" || d.TLSKeyFile != "/tls/tls.key" {
		t.Fatalf("dashboard = %+v", d)
	}
}
