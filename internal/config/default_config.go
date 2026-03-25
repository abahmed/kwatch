package config

func DefaultConfig() *Config {
	return &Config{
		App: App{
			LogFormatter: "text",
		},
		IgnoreFailedGracefulShutdown: true,
		PvcMonitor: PvcMonitor{
			Enabled:   true,
			Interval:  5,
			Threshold: 80,
		},
		NodeMonitor: NodeMonitor{
			Enabled: true,
		},
		Upgrader: Upgrader{
			DisableUpdateCheck: false,
		},
		Telemetry: Telemetry{
			Enabled: false,
		},
		HealthCheck: HealthCheck{
			Enabled: false,
			Port:    8060,
		},
	}
}
