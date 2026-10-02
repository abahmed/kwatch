package config

import "testing"

func TestValidateRejectsStartupRefusedSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"empty required provider field", func(c *Config) {
			c.Alert = map[string]map[string]interface{}{
				"discord": {"webhook": "  "}}
		}, "alert.discord.webhook is required"},
		{"missing required field via alias", func(c *Config) {
			c.Alert = map[string]map[string]interface{}{
				"incident.io": {}}
		}, "alert.incident.io.url is required"},
		{"empty required list", func(c *Config) {
			c.Alert = map[string]map[string]interface{}{
				"sendgrid": {"apiKey": "k", "from": "a@b.c",
					"to": []interface{}{}}}
		}, "alert.sendgrid.to is required"},
		{"health port too high", func(c *Config) {
			c.HealthCheck = HealthCheck{Enabled: true, Port: 70000}
		}, "healthCheck.port must be between 1 and 65535"},
		{"probe url scheme", func(c *Config) {
			c.ActiveProbeMonitor = probeConfig()
			c.ActiveProbeMonitor.HTTP = []HTTPProbeTarget{
				{Name: "a", URL: "ftp://x.test"}}
		}, "unsupported scheme"},
		{"probe url without host", func(c *Config) {
			c.ActiveProbeMonitor = probeConfig()
			c.ActiveProbeMonitor.HTTP = []HTTPProbeTarget{
				{Name: "a", URL: "http://"}}
		}, "not a valid URL"},
		{"tcp address without port", func(c *Config) {
			c.ActiveProbeMonitor = probeConfig()
			c.ActiveProbeMonitor.TCP = []TCPProbeTarget{
				{Name: "a", Address: "db.test"}}
		}, "must be host:port"},
		{"tcp address invalid port", func(c *Config) {
			c.ActiveProbeMonitor = probeConfig()
			c.ActiveProbeMonitor.TCP = []TCPProbeTarget{
				{Name: "a", Address: "db.test:99999"}}
		}, "invalid port"},
		{"heartbeat url scheme", func(c *Config) {
			c.HeartbeatMonitor = HeartbeatMonitor{
				Enabled: true, Interval: 10, URL: "ftp://x.test"}
		}, "heartbeatMonitor.url has unsupported scheme"},
		{"severity keys differ in case", func(c *Config) {
			c.SeverityByReason = map[string]string{
				"OOMKilled": "critical", "oomkilled": "low"}
		}, "differ only in case"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfig()
			test.mutate(cfg)
			assertErrorContains(t, Validate(cfg), test.want)
		})
	}
}

func TestValidateAcceptsCompleteProviderAndProbes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{
		"discord": {"webhook": "https://discord.test/hook"}}
	cfg.ActiveProbeMonitor = probeConfig()
	cfg.ActiveProbeMonitor.HTTP = []HTTPProbeTarget{
		{Name: "a", URL: "https://x.test/healthz"}}
	cfg.ActiveProbeMonitor.TCP = []TCPProbeTarget{
		{Name: "b", Address: "db.test:5432"}}
	if errs := Validate(cfg); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func probeConfig() ActiveProbeMonitor {
	return ActiveProbeMonitor{
		Enabled: true, IntervalSeconds: 30, TimeoutSeconds: 5,
		FailureThreshold: 3,
	}
}
