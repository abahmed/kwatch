package config

import "testing"

func TestValidateDashboardPortMayEqualDisabledHealthPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dashboard.Enabled = true
	cfg.Dashboard.Port = 8060
	cfg.HealthCheck = HealthCheck{Enabled: false, Port: 8060}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	cfg.HealthCheck.Enabled = true
	assertErrorContains(t, Validate(cfg),
		"dashboard.port must not equal healthCheck.port")
}

func TestValidateHeartbeatIntervalMinimum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HeartbeatMonitor = HeartbeatMonitor{
		Enabled: true, URL: "https://x.test/ping", Interval: 1}
	assertErrorContains(t, Validate(cfg),
		"heartbeatMonitor.interval must be 0 (the default) or at least 30")

	for _, ok := range []int{0, MinHeartbeatInterval, 300} {
		cfg.HeartbeatMonitor.Interval = ok
		if errs := Validate(cfg); len(errs) != 0 {
			t.Fatalf("interval %d rejected: %v", ok, errs)
		}
	}
}
