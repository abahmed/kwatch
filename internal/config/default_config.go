package config

import "github.com/abahmed/kwatch/internal/model"

func DefaultConfig() *Config {
	return &Config{
		App: App{LogFormatter: "text"},
		// Opt-in. The payload is small and anonymous -- a per-cluster UUID
		// and the kwatch version, once a week -- but it still leaves the
		// cluster, and a monitoring tool should not be the thing that
		// surprises an operator by making an outbound call they did not ask
		// for. Turning it on logs exactly what is sent.
		Telemetry:                    Telemetry{Enabled: false},
		IgnoreFailedGracefulShutdown: true,
		ReportStartupBaseline:        true,
		MaxRecentLogLines:            50,
		// Detection is event-driven, so an object whose controller stops
		// writing to it is never re-examined -- and the stale sweep then
		// resolves it after Correlation.Window even though it is still
		// broken. A periodic resync re-observes every object, which keeps
		// "still failing" true and makes sustained-minutes monitors fire
		// without needing an unrelated update to arrive.
		ResyncSeconds: 300,
		Workers:       1,
		// Zero leaves cumulative-restart alerting off: a restart count that
		// never resets would alert forever on a pod that recovered weeks ago.
		ContainerRestartThreshold: 0,
		AdaptiveThresholds:        true,
		Maintenance: MaintenanceConfig{
			Enabled:         true,
			Annotation:      "kwatch.io/maintenance",
			UntilAnnotation: "kwatch.io/maintenance-until",
		},
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
			Enabled: false, IntervalSeconds: 60,
			MemoryWarningPercent: 90, MemoryCriticalPercent: 95,
			CPUWarningPercent: 90, CPUCriticalPercent: 100,
		},
		ActiveProbeMonitor: ActiveProbeMonitor{
			IntervalSeconds: 30, TimeoutSeconds: 5,
			FailureThreshold: 3, RecoveryThreshold: 2,
			AutoServices: false,
		},
		KubeletTelemetryMonitor: KubeletTelemetryMonitor{
			Enabled: true, IntervalSeconds: 60, FailureThreshold: 2, RecoveryThreshold: 2, PersistState: true,
			MemoryWarningPercent: 90, MemoryCriticalPercent: 95,
			EphemeralStorageWarningPercent: 90, EphemeralStorageCriticalPercent: 95,
			CPUWarningPercent: 90, CPUCriticalPercent: 100,
			CPUThrottlingWarningPercent: 50, CPUThrottlingCriticalPercent: 75,
			PSIWarningPercent: 20, PSICriticalPercent: 50,
			NetworkErrorRateWarning: 1, NetworkErrorRateCritical: 10,
			RuntimeErrorRateWarning: 1, RuntimeErrorRateCritical: 10,
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
		ControlPlaneMonitor:     ControlPlaneMonitor{Enabled: true, IntervalSeconds: 30, APIServerLatencyWarningMs: 1000, FailureThreshold: 2, RecoveryThreshold: 2},
		IngressMonitor:          IngressMonitor{Enabled: true},
		NetworkPolicyMonitor:    NetworkPolicyMonitor{Enabled: true},
		ClusterResourceMonitor: ClusterResourceMonitor{
			Enabled:          true,
			SustainedMinutes: 10,
			// A lease renews every 10s and node-lifecycle marks a node
			// Unknown at 40s, so 90s means "clearly gone", not a blip.
			NodeLeaseStaleSeconds: 90,
		},
		SmartGrouping: SmartGrouping{
			WindowSeconds:            60,
			NamespaceFanOutThreshold: 3,
		},
		Correlation: Correlation{
			MaxBaseline:       5000,
			Window:            10,
			LifecycleInterval: 1,
			ResolveHoldDown:   300,
			// Deprecated and ignored; kept so an existing YAML that sets it
			// still validates. The post-resolve cooldown is Window.
			CooldownMinutes: 10,
			// Two tiers, because there are two steps above normal: the first
			// crossing raises to high, the second to critical. A third tier had
			// nothing left to escalate to and was silently ignored.
			Escalation: EscalationConfig{Enabled: true, Tiers: []int{3, 10}},
			// Renotify is off until intervalBySeverity is set; the cap below
			// only
			// applies once it is on. Declared here so the default is visible in
			// one place instead of buried in the engine's fallback.
			// A problem nobody has fixed should say so again, on a cadence
			// that matches how urgent it is. Off by default meant a critical
			// incident was announced once and never mentioned again.
			Renotify: RenotifyConfig{
				MaxPerIncident: 3,
				IntervalBySeverity: map[string]int{
					string(model.SeverityCritical): 10,
					string(model.SeverityHigh):     30,
					string(model.SeverityMedium):   60,
					string(model.SeverityWarning):  60,
					"default":                      60,
				},
			},
		},
		AuditLog: AuditLogConfig{Enabled: true, Output: "stdout"},
	}
}
