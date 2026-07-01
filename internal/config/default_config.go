package config

func DefaultConfig() *Config {
	return &Config{
		App:                          App{LogFormatter: "text"},
		IgnoreFailedGracefulShutdown: true,
		ReportStartupBaseline:        false,
		MaxRecentLogLines:            50,
		ResyncSeconds:                0,
		Workers:                      1,
		PvcMonitor:                   PvcMonitor{Enabled: true, Interval: 5, Threshold: 80, CriticalThreshold: 90, ClearThreshold: 75},
		NodeMonitor:                  NodeMonitor{Enabled: true},
		PendingPodMonitor:            PendingPodMonitor{Enabled: true, Threshold: 300},
		RolloutMonitor:               RolloutMonitor{Enabled: true},
		JobMonitor:                   JobMonitor{Enabled: true},
		CronJobMonitor:               CronJobMonitor{Enabled: true},
		DaemonSetMonitor:             DaemonSetMonitor{Enabled: true, SustainedMinutes: 5},
		HpaMonitor:                   HpaMonitor{Enabled: true, SustainedMinutes: 20},
		Upgrader:                     Upgrader{DisableUpdateCheck: false},
		HealthCheck:                  HealthCheck{Enabled: true, Port: 8060, Pprof: false, Diagnostics: false},
		Inhibition:                   Inhibition{NodeSuppressesPods: true},
		StormConfig:                  StormConfig{Enabled: true, Threshold: 10, WindowMinutes: 5, DigestIntervalMinutes: 5},
		LLM:                          LLMConfig{Enabled: true},
		Correlation: Correlation{
			MaxBaseline: 5000,
			Window:      10, 
			LifecycleInterval: 1,
			ResolveHoldDown: 120,
			Escalation:      EscalationConfig{Enabled: true, Tiers: []int{3, 10, 50}},
		},
	}
}
