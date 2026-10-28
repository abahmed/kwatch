package config

import (
	"strings"
	"testing"
)

func TestDashboardEnvironmentOverridesTheFile(t *testing.T) {
	t.Setenv(dashboardEnabledEnv, "true")
	t.Setenv(dashboardPortEnv, "9090")
	t.Setenv(dashboardModeEnv, "Shared")
	cfg := loadConfigText(t, "dashboard:\n  enabled: false\n  port: 8061\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if !d.Enabled || d.Port != 9090 || d.Mode != DashboardShared {
		t.Fatalf("dashboard = %+v, want the environment's values", d)
	}

	t.Setenv(dashboardEnabledEnv, "")
	t.Setenv(dashboardPortEnv, "")
	t.Setenv(dashboardModeEnv, "")
	cfg = loadConfigText(t, "dashboard:\n  enabled: true\n  mode: shared\n")
	d = cfg.Runtime.Lifecycle().Dashboard()
	if !d.Enabled || d.Mode != DashboardShared || d.Port != 8061 {
		t.Fatalf("unset environment must keep the file: %+v", d)
	}
}

func TestDashboardEnvironmentIgnoresBadValuesWithAWarning(t *testing.T) {
	t.Setenv(dashboardPortEnv, "eighty")
	t.Setenv(dashboardModeEnv, "public")
	cfg := loadConfigText(t, "dashboard:\n  enabled: true\n")
	d := cfg.Runtime.Lifecycle().Dashboard()
	if d.Port != 8061 || d.Mode != DashboardLocal {
		t.Fatalf("bad environment changed the settings: %+v", d)
	}
	if got := strings.Join(cfg.envWarnings, "\n"); !strings.Contains(got,
		dashboardPortEnv) || !strings.Contains(got, dashboardModeEnv) {
		t.Fatalf("warnings = %q", got)
	}
}

func TestDashboardModeIsValidated(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dashboard.Enabled = true
	cfg.Dashboard.Mode = "public"
	found := false
	for _, err := range Validate(cfg) {
		if strings.Contains(err.Error(), "dashboard.mode") {
			found = true
		}
	}
	if !found {
		t.Fatal("an unknown dashboard.mode passed validation")
	}
	cfg.Dashboard.Mode = DashboardShared
	for _, err := range Validate(cfg) {
		if strings.Contains(err.Error(), "dashboard") {
			t.Fatalf("shared mode rejected: %v", err)
		}
	}
}
