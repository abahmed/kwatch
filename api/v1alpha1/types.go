package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KwatchConfig is the schema for the kwatch deployment configuration.
type KwatchConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KwatchConfigSpec `json:"spec"`
}

// KwatchConfigSpec defines the desired kwatch configuration.
type KwatchConfigSpec struct {
	// MaxRecentLogLines is the optional max tail log lines in messages.
	MaxRecentLogLines int64 `json:"maxRecentLogLines,omitempty"`

	// IgnoreFailedGracefulShutdown suppresses alerts for containers
	// forcefully killed during shutdown.
	IgnoreFailedGracefulShutdown bool `json:"ignoreFailedGracefulShutdown,omitempty"`

	// Namespaces is an optional list of namespaces to watch or forbid.
	Namespaces []string `json:"namespaces,omitempty"`

	// Reasons is an optional list of reasons to watch or forbid.
	Reasons []string `json:"reasons,omitempty"`

	// IgnoreContainerNames is an optional list of container names to ignore.
	IgnoreContainerNames []string `json:"ignoreContainerNames,omitempty"`

	// IgnorePodNames is an optional list of pod name regex patterns to ignore.
	IgnorePodNames []string `json:"ignorePodNames,omitempty"`

	// IgnoreLogPatterns is an optional list of log regex patterns to ignore.
	IgnoreLogPatterns []string `json:"ignoreLogPatterns,omitempty"`

	// SeverityByOwnerKind maps owner kinds to severity levels.
	SeverityByOwnerKind map[string]string `json:"severityByOwnerKind,omitempty"`

	// PendingPodThreshold is the duration (seconds) a pod can remain
	// in Pending phase before an alert is raised.
	PendingPodThreshold int `json:"pendingPodThreshold,omitempty"`

	// ResyncSeconds is the interval (seconds) for periodic informer resyncs.
	ResyncSeconds int `json:"resyncSeconds,omitempty"`

	// Silences is an optional list of silence rules.
	Silences []SilenceRule `json:"silences,omitempty"`

	// Correlation defines incident dedup and lifecycle settings.
	Correlation CorrelationConfig `json:"correlation,omitempty"`

	// PVC monitor configuration.
	PvcMonitor PvcMonitorConfig `json:"pvcMonitor,omitempty"`

	// Node monitor configuration.
	NodeMonitor NodeMonitorConfig `json:"nodeMonitor,omitempty"`

	// Rollout monitor configuration.
	RolloutMonitor RolloutMonitorConfig `json:"rolloutMonitor,omitempty"`

	// Job monitor configuration.
	JobMonitor JobMonitorConfig `json:"jobMonitor,omitempty"`

	// Heartbeat monitor configuration.
	HeartbeatMonitor HeartbeatMonitorConfig `json:"heartbeatMonitor,omitempty"`

	// Health check configuration.
	HealthCheck HealthCheckConfig `json:"healthCheck,omitempty"`

	// Application-level configuration.
	App AppConfig `json:"app,omitempty"`
}

type CorrelationConfig struct {
	Window            int `json:"window,omitempty"`
	Cooldown          int `json:"cooldown,omitempty"`
	StaleThreshold    int `json:"staleThreshold,omitempty"`
	LifecycleInterval int `json:"lifecycleInterval,omitempty"`
	StartupQuiet      int `json:"startupQuiet,omitempty"`
}

type PvcMonitorConfig struct {
	Enabled   bool    `json:"enabled,omitempty"`
	Interval  int     `json:"interval,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

type NodeMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type RolloutMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type JobMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type HeartbeatMonitorConfig struct {
	Enabled  bool `json:"enabled,omitempty"`
	Interval int  `json:"interval,omitempty"`
}

type HealthCheckConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	Port    int  `json:"port,omitempty"`
}

type AppConfig struct {
	ClusterName           string `json:"clusterName,omitempty"`
	ProxyURL              string `json:"proxyURL,omitempty"`
	DisableStartupMessage bool   `json:"disableStartupMessage,omitempty"`
	LogFormatter          string `json:"logFormatter,omitempty"`
}

type SilenceRule struct {
	Namespaces      []string `json:"namespaces,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
	PodNamePatterns []string `json:"podNamePatterns,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KwatchConfigList contains a list of KwatchConfig.
type KwatchConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KwatchConfig `json:"items"`
}
