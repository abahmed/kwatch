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
		HealthCheck: HealthCheck{
			Enabled: false,
			Port:    8060,
		},
		Correlation: Correlation{
			Window:            10,
			Cooldown:          5,
			StaleThreshold:    15,
			LifecycleInterval: 1,
			StartupQuiet:      30,
		},
		PendingPodThreshold: 300,
		RolloutMonitor: RolloutMonitor{
			Enabled: true,
		},
		JobMonitor: JobMonitor{
			Enabled: true,
		},
	}
}
