package config

func DefaultConfig() *Config {
	return &Config{
		App:                          App{LogFormatter: "text"},
		IgnoreFailedGracefulShutdown: true,
		ReportStartupBaseline:        true,
		MaxRecentLogLines:            50,
		ResyncSeconds:                0,
		Workers:                      1,
		PvcMonitor: PvcMonitor{
			Enabled:           true,
			Interval:          5,
			Threshold:         80,
			CriticalThreshold: 90,
			ClearThreshold:    75,
		},
		NodeMonitor: NodeMonitor{
			Enabled:          true,
			SustainedMinutes: 3,
		},
		ScheduleMonitor: ScheduleMonitor{Enabled: true},
		OomMonitor: OomMonitor{
			Enabled:       true,
			Threshold:     3,
			WindowMinutes: 60,
		},
		PendingPodMonitor: PendingPodMonitor{
			Enabled:   true,
			Threshold: 300,
		},
		NotReadyMonitor: NotReadyMonitor{Enabled: true},
		// Kubernetes itself gives a rollout progressDeadlineSeconds=600 before
		// calling it stuck. Two minutes flagged ordinary rollouts of anything
		// that boots slowly; five still beats a human noticing by a wide
		// margin.
		RolloutMonitor: RolloutMonitor{
			Enabled:          true,
			SustainedMinutes: 5,
		},
		JobMonitor: JobMonitor{Enabled: true},
		CronJobMonitor: CronJobMonitor{
			Enabled:          true,
			SustainedMinutes: 5,
		},
		StatefulSetMonitor: StatefulSetMonitor{
			Enabled:          true,
			SustainedMinutes: 5,
		},
		PdbMonitor: PdbMonitor{
			Enabled:          true,
			SustainedMinutes: 5,
		},
		NodeResourceMonitor: NodeResourceMonitor{
			Enabled:                   true,
			IntervalSeconds:           300,
			CpuWarning:                2.0,
			CpuCritical:               4.0,
			MemWarning:                2.0,
			MemCritical:               4.0,
			FilesystemWarningPercent:  90,
			FilesystemCriticalPercent: 95,
			InodeWarningPercent:       90,
			InodeCriticalPercent:      95,
		},
		RuntimeMetricsMonitor: RuntimeMetricsMonitor{
			Enabled: true, IntervalSeconds: 60,
			MemoryWarningPercent: 90, MemoryCriticalPercent: 100,
			CPUWarningPercent: 90, CPUCriticalPercent: 100,
		},
		ActiveProbeMonitor: ActiveProbeMonitor{
			IntervalSeconds: 30, TimeoutSeconds: 5,
			FailureThreshold: 3, RecoveryThreshold: 2,
		},
		DaemonSetMonitor: DaemonSetMonitor{
			Enabled:          true,
			SustainedMinutes: 5,
		},
		HpaMonitor: HpaMonitor{
			Enabled:          true,
			SustainedMinutes: 20,
		},
		ClusterAutoscalerMonitor: ClusterAutoscalerMonitor{Enabled: true},
		Upgrader:                 Upgrader{DisableUpdateCheck: false},
		HealthCheck: HealthCheck{
			Enabled:     true,
			Port:        8060,
			Pprof:       false,
			Diagnostics: false,
		},
		Inhibition:              Inhibition{NodeSuppressesPods: true},
		ServiceMonitor:          ServiceMonitor{Enabled: true},
		AdmissionWebhookMonitor: AdmissionWebhookMonitor{Enabled: true},
		ControlPlaneMonitor:     ControlPlaneMonitor{Enabled: true},
		IngressMonitor:          IngressMonitor{Enabled: true},
		NetworkPolicyMonitor:    NetworkPolicyMonitor{Enabled: true},
		ClusterResourceMonitor:  ClusterResourceMonitor{Enabled: true, SustainedMinutes: 10},
		SmartGrouping: SmartGrouping{
			WindowSeconds:            60,
			NamespaceFanOutThreshold: 3,
		},
		Correlation: Correlation{
			MaxBaseline:       5000,
			Window:            10,
			LifecycleInterval: 1,
			ResolveHoldDown:   300,
			CooldownMinutes:   10,
			// Two tiers, because there are two steps above normal: the first
			// crossing raises to high, the second to critical. A third tier had
			// nothing left to escalate to and was silently ignored.
			Escalation: EscalationConfig{Enabled: true, Tiers: []int{3, 10}},
			// Renotify is off until intervalBySeverity is set; the cap below
			// only
			// applies once it is on. Declared here so the default is visible in
			// one place instead of buried in the engine's fallback.
			Renotify: RenotifyConfig{MaxPerIncident: 3},
		},
		AuditLog: AuditLogConfig{Enabled: true, Output: "stdout"},
	}
}
