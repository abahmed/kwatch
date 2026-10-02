package config

// DefaultConfig returns the configuration used before any overlay.
func DefaultConfig() *Config {
	return &Config{
		App: App{LogFormatter: "text"},
		// Official builds report a small anonymous adoption heartbeat once a
		// week. Development builds and CI remain suppressed.
		Telemetry: Telemetry{Enabled: true},
		// Informers are event-driven; a periodic resync is a cheap safety net
		// for a watch event lost during an API server disruption.
		ResyncSeconds: 300,
		Maintenance: MaintenanceConfig{
			Enabled:         true,
			Annotation:      "kwatch.io/maintenance",
			UntilAnnotation: "kwatch.io/maintenance-until",
		},
		ActiveProbeMonitor: ActiveProbeMonitor{
			IntervalSeconds: 30, TimeoutSeconds: 5,
			FailureThreshold: 3,
		},
		Upgrader: Upgrader{DisableUpdateCheck: false},
		HealthCheck: HealthCheck{
			Enabled: true,
			Port:    8060,
		},
		AuditLog: AuditLogConfig{Enabled: true, Output: "stdout"},
		Watch:    WatchConfig{Secrets: true},
	}
}
