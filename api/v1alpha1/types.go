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
	IgnoreContainerMessages      []string               `json:"ignoreContainerMessages,omitempty"`
	IgnoreNodeReasons            []string               `json:"ignoreNodeReasons,omitempty"`
	IgnoreNodeMessages           []string               `json:"ignoreNodeMessages,omitempty"`
	IgnoreDisruptionTerminations *bool                  `json:"ignoreDisruptionTerminations,omitempty"`
	NamespaceSelector            string                 `json:"namespaceSelector,omitempty"`
	IncludeEvents                *bool                  `json:"includeEvents,omitempty"`
	IncludeLogs                  *bool                  `json:"includeLogs,omitempty"`
	ContainerRestartThreshold    int                    `json:"containerRestartThreshold,omitempty"`
	ReportStartupBaseline        bool                   `json:"reportStartupBaseline,omitempty"`
	SeverityByOwnerKind          map[string]string      `json:"severityByOwnerKind,omitempty"`
	SeverityByReason             map[string]string      `json:"severityByReason,omitempty"`
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
	Upgrader                     map[string]interface{} `json:"upgrader,omitempty"`
	Alert                        map[string]interface{} `json:"alert,omitempty"`
	ScheduleMonitor              MonitorConfig          `json:"scheduleMonitor,omitempty"`
	OomMonitor                   MonitorConfig          `json:"oomMonitor,omitempty"`
	PendingPodMonitor            MonitorConfig          `json:"pendingPodMonitor,omitempty"`
	NotReadyMonitor              MonitorConfig          `json:"notReadyMonitor,omitempty"`
	StatefulSetMonitor           MonitorConfig          `json:"statefulSetMonitor,omitempty"`
	PdbMonitor                   MonitorConfig          `json:"pdbMonitor,omitempty"`
	NodeResourceMonitor          MonitorConfig          `json:"nodeResourceMonitor,omitempty"`
	ClusterAutoscalerMonitor     MonitorConfig          `json:"clusterAutoscalerMonitor,omitempty"`
	HpaMonitor                   MonitorConfig          `json:"hpaMonitor,omitempty"`
	TlsMonitor                   MonitorConfig          `json:"tlsMonitor,omitempty"`
	ServiceMonitor               MonitorConfig          `json:"serviceMonitor,omitempty"`
	AdmissionWebhookMonitor      MonitorConfig          `json:"admissionWebhookMonitor,omitempty"`
	ControlPlaneMonitor          MonitorConfig          `json:"controlPlaneMonitor,omitempty"`
	IngressMonitor               MonitorConfig          `json:"ingressMonitor,omitempty"`
	NetworkPolicyMonitor         MonitorConfig          `json:"networkPolicyMonitor,omitempty"`
	ClusterResourceMonitor       MonitorConfig          `json:"clusterResourceMonitor,omitempty"`
	SmartGrouping                MonitorConfig          `json:"smartGrouping,omitempty"`
	Inhibition                   MonitorConfig          `json:"inhibition,omitempty"`
	Templates                    map[string]string      `json:"templates,omitempty"`
	Runbooks                     map[string]string      `json:"runbooks,omitempty"`
	AuditLog                     AuditLogConfig         `json:"auditLog,omitempty"`
	Workers                      int                    `json:"workers,omitempty"`
}

// MonitorConfig is intentionally open-ended because monitor options evolve
// independently from the CRD version. The CRD remains the source of truth for
// validation while typed clients can round-trip newly added options safely.
type MonitorConfig map[string]interface{}

type AuditLogConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Output  string `json:"output,omitempty"`
}

type CorrelationConfig struct {
	Window            int           `json:"window,omitempty"`
	Cooldown          int           `json:"cooldown,omitempty"`
	StaleThreshold    int           `json:"staleThreshold,omitempty"`
	LifecycleInterval int           `json:"lifecycleInterval,omitempty"`
	ResolveHoldDown   int           `json:"resolveHoldDown,omitempty"`
	CooldownMinutes   int           `json:"cooldownMinutes,omitempty"`
	MaxBaseline       int           `json:"maxBaseline,omitempty"`
	Escalation        MonitorConfig `json:"escalation,omitempty"`
	Renotify          MonitorConfig `json:"renotify,omitempty"`
}

type PvcMonitorConfig struct {
	Enabled           bool    `json:"enabled,omitempty"`
	Interval          int     `json:"interval,omitempty"`
	Threshold         float64 `json:"threshold,omitempty"`
	CriticalThreshold float64 `json:"criticalThreshold,omitempty"`
	ClearThreshold    float64 `json:"clearThreshold,omitempty"`
}

type NodeMonitorConfig struct {
	Enabled          bool `json:"enabled,omitempty"`
	SustainedMinutes int  `json:"sustainedMinutes,omitempty"`
}

type RolloutMonitorConfig struct {
	Enabled          bool `json:"enabled,omitempty"`
	SustainedMinutes int  `json:"sustainedMinutes,omitempty"`
}

type DaemonSetMonitorConfig struct {
	Enabled          bool `json:"enabled,omitempty"`
	SustainedMinutes int  `json:"sustainedMinutes,omitempty"`
}

type JobMonitorConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type CronJobMonitorConfig struct {
	Enabled          bool `json:"enabled,omitempty"`
	SustainedMinutes int  `json:"sustainedMinutes,omitempty"`
}

type HeartbeatMonitorConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Interval int    `json:"interval,omitempty"`
	URL      string `json:"url,omitempty"`
}

type HealthCheckConfig struct {
	Enabled          bool   `json:"enabled,omitempty"`
	Port             int    `json:"port,omitempty"`
	Pprof            bool   `json:"pprof,omitempty"`
	Diagnostics      bool   `json:"diagnostics,omitempty"`
	DiagnosticsToken string `json:"diagnosticsToken,omitempty"`
}

type AppConfig struct {
	ClusterName           string `json:"clusterName,omitempty"`
	ProxyURL              string `json:"proxyURL,omitempty"`
	DisableStartupMessage bool   `json:"disableStartupMessage,omitempty"`
	LogFormatter          string `json:"logFormatter,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
	CABundlePath          string `json:"caBundlePath,omitempty"`
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
