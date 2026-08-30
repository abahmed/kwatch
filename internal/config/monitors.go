package config

type ClusterAutoscalerMonitor struct {
	// Enabled toggles cluster-autoscaler event monitoring.
	Enabled bool `yaml:"enabled"`
}

// HpaMonitor configures HPA-maxed-out detection.
type HpaMonitor struct {
	// Enabled if set to true, it will watch HPAs for maxed-out replicas.
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long the HPA must be maxed before alerting.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// TlsMonitor configures TLS certificate expiry monitoring.
type TlsMonitor struct {
	// Enabled if set to true, it will monitor TLS secret certificates for expiry.
	Enabled bool `yaml:"enabled"`

	// Threshold is the number of days before expiry at which to alert. Default 30.
	Threshold int `yaml:"threshold"`

	// CriticalThreshold is the number of days before expiry at which severity
	// is raised to "high". Default 3.
	CriticalThreshold int `yaml:"criticalThreshold"`
}

// ServiceMonitor configures service endpoint health monitoring.
type ServiceMonitor struct {
	// Enabled if set to true, it will monitor service endpoint health.
	Enabled bool `yaml:"enabled"`
}

// AdmissionWebhookMonitor configures admission webhook failure monitoring.
type AdmissionWebhookMonitor struct {
	// Enabled if set to true, it will monitor admission webhook failures.
	Enabled bool `yaml:"enabled"`
}

// ControlPlaneMonitor configures control-plane health monitoring.
type ControlPlaneMonitor struct {
	// Enabled if set to true, it will monitor control-plane health.
	Enabled bool `yaml:"enabled"`
}

// IngressMonitor configures ingress backend health monitoring.
type IngressMonitor struct {
	// Enabled if set to true, it will monitor ingress backend health.
	Enabled bool `yaml:"enabled"`
}

// NetworkPolicyMonitor configures network policy issue monitoring.
type NetworkPolicyMonitor struct {
	// Enabled if set to true, it will monitor network policy issues.
	Enabled bool `yaml:"enabled"`
}

// ClusterResourceMonitor configures status checks for cluster-level resource
// exhaustion and lifecycle failures that are not represented by Pod events.
type ClusterResourceMonitor struct {
	// Enabled enables ResourceQuota and stuck Namespace detection.
	Enabled bool `yaml:"enabled"`
	// SustainedMinutes is the minimum age of a terminating Namespace before it
	// is reported as stuck. Zero uses the built-in default.
	SustainedMinutes int `yaml:"sustainedMinutes"`
	// NodeLeaseStaleSeconds is how long a node lease may go without a renew
	// time before it is reported. Zero uses 90 seconds.
	NodeLeaseStaleSeconds int `yaml:"nodeLeaseStaleSeconds"`
}

type PvcMonitor struct {
	// Enabled if set to true, it will check pvc usage periodically
	// By default, this value is true
	Enabled bool `yaml:"enabled"`

	// Interval is the frequency (in minutes) to check pvc usage in the cluster
	// By default, this value is 5
	Interval int `yaml:"interval"`

	// Threshold is the percentage of accepted pvc usage. if current usage
	// exceeds this value, it will send a notification (warn tier).
	// By default, this value is 80
	Threshold float64 `yaml:"threshold"`

	// CriticalThreshold is the percentage above which severity is "high".
	// By default, this value is 90
	CriticalThreshold float64 `yaml:"criticalThreshold"`

	// ClearThreshold is the percentage below which an alerted PVC is resolved.
	// Must be <= Threshold. 0 (default 75) means no hysteresis — uses Threshold.
	ClearThreshold float64 `yaml:"clearThreshold"`
}

// NodeMonitor confing struct
type NodeMonitor struct {
	// Enabled if set to true, it will enable node watcher
	// By default, this value is true
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long a node condition (MemoryPressure,
	// DiskPressure, PIDPressure, NetworkUnavailable) must persist before
	// alerting, to avoid noise from brief metric spikes. Default 3.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// HeartbeatMonitor config for dead man's switch
type HeartbeatMonitor struct {
	// Enabled if set to true, a periodic heartbeat ping is sent.
	Enabled bool `yaml:"enabled"`

	// Interval is the frequency (in seconds) between pings. Default 300 (5 min).
	Interval int `yaml:"interval"`

	// URL is the external endpoint to ping (e.g. Healthchecks.io).
	// When set, a GET request is sent every interval; no response means the
	// external monitor pages.
	URL string `yaml:"url"`
}

// RolloutMonitor config struct
type RolloutMonitor struct {
	// Enabled if set to true, it will watch Deployments for stuck rollouts
	// By default, this value is true
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long deployment pods must be unavailable
	// before alerting, to avoid noise from rolling updates. Default 2.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// JobMonitor config struct
type JobMonitor struct {
	// Enabled if set to true, it will watch Jobs for failures
	// By default, this value is true
	Enabled bool `yaml:"enabled"`
}

// StatefulSetMonitor configures rollout-stuck detection for StatefulSets.
type StatefulSetMonitor struct {
	// Enabled if set to true, it will watch StatefulSets for stuck rollouts.
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long the StatefulSet must be unavailable before
	// alerting, to avoid noise from rolling updates and brief node blips.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// PdbMonitor configures PDB violation detection.
type PdbMonitor struct {
	// Enabled if set to true, it will watch PodDisruptionBudgets for violations.
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long the PDB must be blocking before alerting.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// NodeResourceMonitor configures node resource overcommit prediction.
type NodeResourceMonitor struct {
	// Enabled if set to true, periodically checks node resource overcommit levels.
	Enabled bool `yaml:"enabled"`

	// IntervalSeconds is how often to check node resource usage. Default 300.
	IntervalSeconds int `yaml:"intervalSeconds"`

	// CpuWarning is the CPU overcommit ratio for warning (e.g. 2.0 = 200%).
	CpuWarning float64 `yaml:"cpuWarning"`

	// CpuCritical is the CPU overcommit ratio for critical.
	CpuCritical float64 `yaml:"cpuCritical"`

	// MemWarning is the memory overcommit ratio for warning.
	MemWarning float64 `yaml:"memWarning"`

	// MemCritical is the memory overcommit ratio for critical.
	MemCritical float64 `yaml:"memCritical"`
}

// RuntimeMetricsMonitor reads metrics.k8s.io when a Metrics Server is
// available and compares actual container usage with declared limits.
type RuntimeMetricsMonitor struct {
	Enabled               bool `yaml:"enabled"`
	IntervalSeconds       int  `yaml:"intervalSeconds"`
	MemoryWarningPercent  int  `yaml:"memoryWarningPercent"`
	MemoryCriticalPercent int  `yaml:"memoryCriticalPercent"`
	CPUWarningPercent     int  `yaml:"cpuWarningPercent"`
	CPUCriticalPercent    int  `yaml:"cpuCriticalPercent"`
}

// DaemonSetMonitor configures rollout-stuck detection for DaemonSets.
type DaemonSetMonitor struct {
	// Enabled if set to true, it will watch DaemonSets for stuck rollouts.
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long the DaemonSet must be unavailable before
	// alerting, to avoid noise from rolling updates and brief node blips.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

// CronJobMonitor configures failed/suspended CronJob detection.
type CronJobMonitor struct {
	// Enabled if set to true, it will watch CronJobs for failures or suspension.
	Enabled bool `yaml:"enabled"`

	// SustainedMinutes is how long the CronJob must be suspended before alerting,
	// to avoid noise from intentional suspension during incident response. Default 5.
	SustainedMinutes int `yaml:"sustainedMinutes"`
}

type ScheduleMonitor struct {
	// Enabled if set to true, adds scheduling duration to Unschedulable hints.
	Enabled bool `yaml:"enabled"`
}

// OomMonitor configures OOM pattern / memory leak detection.
type OomMonitor struct {
	// Enabled if set to true, tracks OOM frequency per container.
	Enabled bool `yaml:"enabled"`

	// Threshold is the number of OOMs within WindowMinutes to flag as repeating.
	Threshold int `yaml:"threshold"`

	// WindowMinutes is the sliding window for OOM tracking.
	WindowMinutes int `yaml:"windowMinutes"`
}

// PendingPodMonitor config struct
type PendingPodMonitor struct {
	// Enabled if set to true, it will watch pods stuck in Pending phase
	Enabled bool `yaml:"enabled"`

	// Threshold is the duration (in seconds) a pod can remain
	// in Pending phase before an alert is raised. Default 300 (5 min).
	Threshold int `yaml:"threshold"`
}

// NotReadyMonitor configures sustained not-ready pod detection.
// It alerts when a pod has been not ready (e.g. failing readiness probe)
// for longer than the built-in threshold even though its containers are
// running and have not crashed — a case the container detectors skip.
type NotReadyMonitor struct {
	// Enabled if set to true, it will watch pods stuck not ready.
	Enabled bool `yaml:"enabled"`
}
