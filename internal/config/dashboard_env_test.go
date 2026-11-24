package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestDashboardSessionSettingsFromFileAndEnvironment(t *testing.T) {
	cfg := loadConfigText(t, "dashboard:\n  enabled: true\n"+
		"  sessionLifetime: 72h\n  signInHelp: Ask the ops channel\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if d.Lifetime() != 72*time.Hour || d.SignInHelp != "Ask the ops channel" {
		t.Fatalf("dashboard = %+v, want the file's session settings", d)
	}
	t.Setenv(dashboardLifeEnv, "168h")
	t.Setenv(dashboardHelpEnv, "See the wiki")
	d = loadConfigText(t, "dashboard:\n  enabled: true\n  "+
		"sessionLifetime: 72h\n").Runtime.Lifecycle().Dashboard()
	if d.Lifetime() != 168*time.Hour || d.SignInHelp != "See the wiki" {
		t.Fatalf("dashboard = %+v, want the environment's", d)
	}
}

func TestValidateDashboardSession(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dashboard.Enabled = true
	for _, ok := range []string{"", "15m", "12h", "168h"} {
		cfg.Dashboard.SessionLifetime = ok
		if errs := Validate(cfg); len(errs) != 0 {
			t.Fatalf("sessionLifetime %q rejected: %v", ok, errs)
		}
	}
	for _, bad := range []string{"14m", "169h", "8d", "soon", "-1h"} {
		cfg.Dashboard.SessionLifetime = bad
		assertErrorContains(t, Validate(cfg), "dashboard.sessionLifetime")
		if cfg.Dashboard.Lifetime() != 0 {
			t.Errorf("%q: Lifetime() must fall back to the default", bad)
		}
	}
	cfg.Dashboard.SessionLifetime = ""
	cfg.Dashboard.SignInHelp = "line one\nline two <b>"
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("plain help rejected: %v", errs)
	}
	cfg.Dashboard.SignInHelp = strings.Repeat("é", MaxSignInHelp+1)
	assertErrorContains(t, Validate(cfg), "at most 500")
	cfg.Dashboard.SignInHelp = "bell\a"
	assertErrorContains(t, Validate(cfg), "without control characters")
}
