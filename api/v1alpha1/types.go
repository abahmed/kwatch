package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KwatchConfig is the schema for the kwatch deployment configuration.
type KwatchConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec KwatchConfigSpec `json:"spec"`
}

// KwatchConfigSpec defines the desired kwatch configuration. It mirrors
// the configuration file; see the configuration reference for each field.
type KwatchConfigSpec struct {
	App                     AppConfig              `json:"app,omitempty"`
	Telemetry               TelemetryConfig        `json:"telemetry,omitempty"`
	Upgrader                map[string]interface{} `json:"upgrader,omitempty"`
	HeartbeatMonitor        HeartbeatMonitorConfig `json:"heartbeatMonitor,omitempty"`
	HealthCheck             HealthCheckConfig      `json:"healthCheck,omitempty"`
	Maintenance             MaintenanceConfig      `json:"maintenance,omitempty"`
	Namespaces              []string               `json:"namespaces,omitempty"`
	Reasons                 []string               `json:"reasons,omitempty"`
	NamespaceSelector       string                 `json:"namespaceSelector,omitempty"`
	IgnoreContainerNames    []string               `json:"ignoreContainerNames,omitempty"`
	IgnorePodNames          []string               `json:"ignorePodNames,omitempty"`
	IgnoreContainerMessages []string               `json:"ignoreContainerMessages,omitempty"`
	IgnoreNodeReasons       []string               `json:"ignoreNodeReasons,omitempty"`
	IgnoreNodeMessages      []string               `json:"ignoreNodeMessages,omitempty"`
	Silences                []SilenceRule          `json:"silences,omitempty"`
	ResyncSeconds           int                    `json:"resyncSeconds,omitempty"`
	SeverityByOwnerKind     map[string]string      `json:"severityByOwnerKind,omitempty"`
	SeverityByReason        map[string]string      `json:"severityByReason,omitempty"`
	ActiveProbeMonitor      MonitorConfig          `json:"activeProbeMonitor,omitempty"`
	Crd                     MonitorConfig          `json:"crd,omitempty"`
	Templates               map[string]string      `json:"templates,omitempty"`
	Runbooks                map[string]string      `json:"runbooks,omitempty"`
	AuditLog                AuditLogConfig         `json:"auditLog,omitempty"`
}

// MonitorConfig is intentionally open-ended because monitor options evolve
// independently from the CRD version. The CRD remains the source of truth for
// validation while typed clients can round-trip newly added options safely.
type MonitorConfig map[string]interface{}

type MaintenanceConfig struct {
	Enabled         bool   `json:"enabled,omitempty"`
	Annotation      string `json:"annotation,omitempty"`
	UntilAnnotation string `json:"untilAnnotation,omitempty"`
}

type TelemetryConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type AuditLogConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Output  string `json:"output,omitempty"`
}

// HeartbeatMonitorConfig omits the URL: it can carry a token, so it is
// only accepted from the mounted configuration file.
type HeartbeatMonitorConfig struct {
	Enabled  bool `json:"enabled,omitempty"`
	Interval int  `json:"interval,omitempty"`
}

type HealthCheckConfig struct {
	Enabled     bool `json:"enabled,omitempty"`
	Port        int  `json:"port,omitempty"`
	Pprof       bool `json:"pprof,omitempty"`
	Diagnostics bool `json:"diagnostics,omitempty"`
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
	ContainerMessages []string `json:"containerMessages,omitempty"`
	EventMessages     []string `json:"eventMessages,omitempty"`
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
