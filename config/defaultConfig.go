package config

func DefaultConfig() *Config {
	return &Config{
		PvcMonitor: PvcMonitor{
			Enabled:   true,
			Interval:  5,
			Threshold: 80,
		},
	}
}
