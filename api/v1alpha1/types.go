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
	MaxRecentLogLines            int64                  `json:"maxRecentLogLines,omitempty"`
	IgnoreFailedGracefulShutdown bool                   `json:"ignoreFailedGracefulShutdown,omitempty"`
	Namespaces                   []string               `json:"namespaces,omitempty"`
	Reasons                      []string               `json:"reasons,omitempty"`
	IgnoreContainerNames         []string               `json:"ignoreContainerNames,omitempty"`
	IgnorePodNames               []string               `json:"ignorePodNames,omitempty"`
	IgnoreLogPatterns            []string               `json:"ignoreLogPatterns,omitempty"`
	SeverityByOwnerKind          map[string]string      `json:"severityByOwnerKind,omitempty"`
	PendingPodThreshold          int                    `json:"pendingPodThreshold,omitempty"`
	ResyncSeconds                int                    `json:"resyncSeconds,omitempty"`
	Silences                     []SilenceRule          `json:"silences,omitempty"`
	Correlation                  CorrelationConfig      `json:"correlation,omitempty"`
	PvcMonitor                   PvcMonitorConfig       `json:"pvcMonitor,omitempty"`
	NodeMonitor                  NodeMonitorConfig      `json:"nodeMonitor,omitempty"`
	RolloutMonitor               RolloutMonitorConfig   `json:"rolloutMonitor,omitempty"`
	DaemonSetMonitor             DaemonSetMonitorConfig `json:"daemonSetMonitor,omitempty"`
	JobMonitor                   JobMonitorConfig       `json:"jobMonitor,omitempty"`
	CronJobMonitor               CronJobMonitorConfig   `json:"cronJobMonitor,omitempty"`
	HeartbeatMonitor             HeartbeatMonitorConfig `json:"heartbeatMonitor,omitempty"`
	HealthCheck                  HealthCheckConfig      `json:"healthCheck,omitempty"`
	App                          AppConfig              `json:"app,omitempty"`
	Workers                      int                    `json:"workers,omitempty"`
}

type CorrelationConfig struct {
	Window            int `json:"window,omitempty"`
	Cooldown          int `json:"cooldown,omitempty"`
	StaleThreshold    int `json:"staleThreshold,omitempty"`
	LifecycleInterval int `json:"lifecycleInterval,omitempty"`
}

type PvcMonitorConfig struct {
	Enabled           bool    `json:"enabled,omitempty"`
	Interval          int     `json:"interval,omitempty"`
	Threshold         float64 `json:"threshold,omitempty"`
	CriticalThreshold float64 `json:"criticalThreshold,omitempty"`
}

type NodeMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type RolloutMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type DaemonSetMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type JobMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type CronJobMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type HeartbeatMonitorConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Interval int    `json:"interval,omitempty"`
	URL      string `json:"url,omitempty"`
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
	Namespaces        []string `json:"namespaces,omitempty"`
	Reasons           []string `json:"reasons,omitempty"`
	PodNamePatterns   []string `json:"podNamePatterns,omitempty"`
	ContainerNames    []string `json:"containerNames,omitempty"`
	LogPatterns       []string `json:"logPatterns,omitempty"`
	ContainerMessages []string `json:"containerMessages,omitempty"`
	NodeReasons       []string `json:"nodeReasons,omitempty"`
	NodeMessages      []string `json:"nodeMessages,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KwatchConfigList contains a list of KwatchConfig.
type KwatchConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KwatchConfig `json:"items"`
}
