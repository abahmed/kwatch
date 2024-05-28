package config

func DefaultConfig() *Config {
	return &Config{
		IgnoreFailedGracefulShutdown: true,
		PvcMonitor: PvcMonitor{
			Enabled:   true,
			Interval:  5,
			Threshold: 80,
		},
	}
}
