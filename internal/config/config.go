package config

import (
	"regexp"
	"time"
)

type Config struct {
	// App general configuration
	App App `yaml:"app"`

	// Upgrader configuration
	Upgrader Upgrader `yaml:"upgrader"`

	// ContainerRestartThreshold, when > 0, opens an incident for any container
	// whose cumulative restart count reaches this threshold, even while
	// currently Running. Default 0 (disabled).
	ContainerRestartThreshold int `yaml:"containerRestartThreshold"`

	// PvcMonitor configuration
	PvcMonitor PvcMonitor `yaml:"pvcMonitor"`

	// HeartbeatMonitor configuration
	HeartbeatMonitor HeartbeatMonitor `yaml:"heartbeatMonitor"`

	// NodeMonitor configuration
	NodeMonitor NodeMonitor `yaml:"nodeMonitor"`

	// HealthCheck configuration
	HealthCheck HealthCheck `yaml:"healthCheck"`

	// Correlation configuration for incident dedup/grouping
	Correlation Correlation `yaml:"correlation"`

	// ReportStartupBaseline if true (default), emits a single informational
	// notification at startup summarizing pre-existing issues that are
	// suppressed from per-incident alerts by the baseline.
	ReportStartupBaseline bool `yaml:"reportStartupBaseline"`

	// MaxRecentLogLines optional max tail log lines in messages,
	// if it's not provided it will get all log lines
	MaxRecentLogLines int64 `yaml:"maxRecentLogLines"`

	// IgnoreFailedGracefulShutdown if set to true, containers which are
	// forcefully killed during shutdown (as their graceful shutdown failed)
	// are not reported as error
	IgnoreFailedGracefulShutdown bool `yaml:"ignoreFailedGracefulShutdown"`

	// Namespaces is an optional list of namespaces that you want to watch or
	// forbid, if it's not provided it will watch all namespaces.
	// If you want to forbid a namespace, configure it with !<namespace name>
	// You can either set forbidden namespaces or allowed, not both
	Namespaces []string `yaml:"namespaces"`

	// Reasons is an  optional list of reasons that you want to watch or forbid,
	// if it's not provided it will watch all reasons.
	// If you want to forbid a reason, configure it with !<reason>
	// You can either set forbidden reasons or allowed, not both
	Reasons []string `yaml:"reasons"`

	// IgnoreContainerNames optional list of container names to ignore
	IgnoreContainerNames []string `yaml:"ignoreContainerNames"`

	// IgnorePodNames optional list of pod name regexp patterns to ignore
	IgnorePodNames []string `yaml:"ignorePodNames"`

	// IgnoreLogPatterns optional list of regexp patterns to ignore
	IgnoreLogPatterns []string `yaml:"ignoreLogPatterns"`

	// IgnoreContainerMessages optional list of substring patterns; if a
	// container status Waiting/Terminated Message contains any entry the
	// incident is suppressed.
	IgnoreContainerMessages []string `yaml:"ignoreContainerMessages"`

	// IgnoreDisruptionTerminations if true (default), pods with a
	// DeletionTimestamp or DisruptionTarget condition (eviction, scale-down,
	// preemption, taint-based termination, etc.) are not alerted.
	IgnoreDisruptionTerminations *bool `yaml:"ignoreDisruptionTerminations"`

	// NamespaceSelector is a Kubernetes label selector to discover namespaces
	// to watch. Mutually exclusive with Namespaces.
	NamespaceSelector string `yaml:"namespaceSelector"`

	// IncludeEvents if false, events section is omitted from alert messages.
	IncludeEvents *bool `yaml:"includeEvents"`

	// IncludeLogs if false, logs section is omitted from alert messages.
	IncludeLogs *bool `yaml:"includeLogs"`

	// Alert is a map contains a map of each provider configuration
	// e.g. {"slack": {"webhook": "URL"}}
	Alert map[string]map[string]interface{} `yaml:"alert"`

	// AllowedNamespaces, ForbiddenNamespaces are calculated internally
	// after populating Namespaces configuration
	AllowedNamespaces   []string
	ForbiddenNamespaces []string

	// AllowedReasons, ForbiddenReasons are calculated internally after
	// populating Reasons configuration
	AllowedReasons   []string
	ForbiddenReasons []string

	// Patterns are compiled from IgnorePodNames after populating
	// IgnorePodNames configuration
	IgnorePodNamePatterns []*regexp.Regexp

	// Patterns are compiled from IgnoreLogPatterns after populating
	// IgnoreLogPatterns configuration
	IgnoreLogPatternsCompiled []*regexp.Regexp

	// IgnoreNodeReasons is an optional list of node reasons for which alerting should be skipped
	IgnoreNodeReasons []string `yaml:"ignoreNodeReasons"`
	// IgnoreNodeMessages is an optional list of node messages for which alerting should be skipped
	IgnoreNodeMessages []string `yaml:"ignoreNodeMessages"`

	// ResyncSeconds is the interval (in seconds) for periodic informer resyncs.
	// If 0, no periodic resync occurs (event-driven only).
	// On large clusters with 200+ pods, raise Workers (below) to match;
	ResyncSeconds int `yaml:"resyncSeconds"`

	// SeverityByOwnerKind maps owner kinds to severity levels.
	// e.g. {"StatefulSet": "high", "DaemonSet": "low"}
	// Default: StatefulSet → "high", everything else → "normal"
	SeverityByOwnerKind map[string]string `yaml:"severityByOwnerKind"`

	// SeverityByReason maps event reasons to severity levels, checked before
	// owner-kind. e.g. {"OOMKilled": "high", "CrashLoopBackOff": "high"}
	SeverityByReason map[string]string `yaml:"severityByReason"`

	// ScheduleMonitor configures scheduling delay diagnostics.
	ScheduleMonitor ScheduleMonitor `yaml:"scheduleMonitor"`

	// OomMonitor configures OOM pattern / memory leak detection.
	OomMonitor OomMonitor `yaml:"oomMonitor"`

	// PendingPodMonitor configures Pending-phase pod detection.
	PendingPodMonitor PendingPodMonitor `yaml:"pendingPodMonitor"`

	// NotReadyMonitor configures sustained not-ready pod detection.
	NotReadyMonitor NotReadyMonitor `yaml:"notReadyMonitor"`

	// RolloutMonitor configures stuck-rollout detection for Deployments.
	RolloutMonitor RolloutMonitor `yaml:"rolloutMonitor"`

	// JobMonitor configures failed/suspended Job detection.
	JobMonitor JobMonitor `yaml:"jobMonitor"`

	// StatefulSetMonitor configures rollout-stuck detection for StatefulSets.
	StatefulSetMonitor StatefulSetMonitor `yaml:"statefulSetMonitor"`

	// PdbMonitor configures PDB violation detection.
	PdbMonitor PdbMonitor `yaml:"pdbMonitor"`

	// NodeResourceMonitor configures node resource overcommit prediction.
	NodeResourceMonitor NodeResourceMonitor `yaml:"nodeResourceMonitor"`

	// DaemonSetMonitor configures rollout-stuck detection for DaemonSets.
	DaemonSetMonitor DaemonSetMonitor `yaml:"daemonSetMonitor"`

	// CronJobMonitor configures failed/suspended CronJob detection.
	CronJobMonitor CronJobMonitor `yaml:"cronJobMonitor"`

	// ClusterAutoscalerMonitor configures cluster-autoscaler event monitoring.
	ClusterAutoscalerMonitor ClusterAutoscalerMonitor `yaml:"clusterAutoscalerMonitor"`

	// HpaMonitor configures HPA-maxed-out detection.
	HpaMonitor HpaMonitor `yaml:"hpaMonitor"`

	// TlsMonitor configures TLS certificate expiry monitoring.
	TlsMonitor TlsMonitor `yaml:"tlsMonitor"`

	// ServiceMonitor configures service endpoint health monitoring.
	ServiceMonitor ServiceMonitor `yaml:"serviceMonitor"`

	// AdmissionWebhookMonitor configures admission webhook failure monitoring.
	AdmissionWebhookMonitor AdmissionWebhookMonitor `yaml:"admissionWebhookMonitor"`

	// ControlPlaneMonitor configures control-plane health monitoring.
	ControlPlaneMonitor ControlPlaneMonitor `yaml:"controlPlaneMonitor"`

	// IngressMonitor configures ingress backend health monitoring.
	IngressMonitor IngressMonitor `yaml:"ingressMonitor"`

	// NetworkPolicyMonitor configures network policy issue monitoring.
	NetworkPolicyMonitor NetworkPolicyMonitor `yaml:"networkPolicyMonitor"`

	// Silences is an optional list of silence rules that suppress matching incidents.
	Silences []SilenceRule `yaml:"silences"`

	// SuppressionIndex is compiled from both Silences and deprecated ignore*
	// fields for efficient detect-time lookup. Populated by LoadConfig.
	Suppression SuppressionIndex

	// WatchStartTime is set once at startup and used by filters to measure
	// resource age relative to when kwatch began watching (not pod birth).
	WatchStartTime time.Time `yaml:"-"`

	// Workers is the number of concurrent reconcile workers per queue.
	// Default 1. Raising it increases throughput on large clusters; alert
	// ordering across pods becomes non-deterministic (engine dedup unaffected).
	Workers int `yaml:"workers"`

	// Inhibition configures suppression rules between monitors.
	Inhibition Inhibition `yaml:"inhibition"`

	// SmartGrouping configures coalescing same-reason incidents across
	// owners into a single notification within a time window.
	SmartGrouping SmartGrouping `yaml:"smartGrouping"`

	// CrdConfig configures the KwatchConfig CRD watcher.
	CrdConfig CrdConfig `yaml:"crd"`

	// Templates maps incident reason (lowercased) to Go text/template string.
	// Available template keys: {{.Incident.Key}}, {{.Incident.Reason}},
	// {{.Action}}, {{.Message}}. Missing keys render as empty string.
	Templates map[string]string `yaml:"templates"`

	// Runbooks maps Kubernetes event reasons to documentation URLs.
	// When a reason matches, the URL is appended to the incident hint.
	Runbooks map[string]string `yaml:"runbooks"`

	// AuditLog configures structured JSON audit logging for all incidents.
	AuditLog AuditLogConfig `yaml:"auditLog"`
}

// KnownProviders is the canonical set of known alert provider names.
// Both alert.Init and config validation reference this to prevent drift.
var KnownProviders = map[string]bool{
	"slack": true, "pagerduty": true, "discord": true, "telegram": true,
	"teams": true, "email": true, "rocketchat": true, "mattermost": true,
	"opsgenie": true, "matrix": true, "dingtalk": true, "feishu": true,
	"webhook": true, "zenduty": true, "googlechat": true,
	"gotify": true, "ntfy": true, "pushover": true, "webex": true,
	"github": true, "line": true,
	"gitlab": true, "gitea": true, "zapier": true, "n8n": true, "ifttt": true,
	"teamsworkflow": true, "zulip": true, "homeassistant": true,
	"splunk": true, "datadog": true,
	"newrelic": true, "clickup": true, "ilert": true,
	"incidentio": true, "incident.io": true, "squadcast": true, "signl4": true,
	"twilio": true, "vonage": true, "plivo": true,
	"messagebird": true, "signal": true, "sendgrid": true, "ses": true,
	"sns": true, "jira": true, "wecom": true, "splunkoncall": true,
	"mailgun": true, "resend": true, "goalert": true, "alerta": true,
	"threema": true, "flock": true, "pushbullet": true, "sensugo": true,
}

// Inhibition configures cross-monitor suppression rules.
type Inhibition struct {
	// NodeSuppressesPods if true, pod incidents on a node with an active
	// node incident are suppressed to reduce noise. Default true.
	NodeSuppressesPods bool `yaml:"nodeSuppressesPods"`
}

// ClusterAutoscalerMonitor configures cluster-autoscaler event monitoring.
// Watches cluster-autoscaler events (TriggeredScaleUp, FailedToScaleUp,
// ScaleDown, etc.) and alerts when the autoscaler cannot scale or
// detects resource constraints.

// App confing struct
type App struct {
	// ProxyURL to be used in outgoing http(s) requests except Kubernetes
	// requests to cluster
	ProxyURL string `yaml:"proxyURL"`

	// ClusterName to used in notifications to indicate which cluster has
	// issue
	ClusterName string `yaml:"clusterName"`

	// DisableUpdateCheck if set to true, welcome message will not be
	// sent to configured notification channels
	DisableStartupMessage bool `yaml:"disableStartupMessage"`

	// LogFormatter used for setting custom formatter when app prints logs
	LogFormatter string `yaml:"logFormatter"`

	// InsecureSkipTLSVerify if true, skips TLS certificate verification
	// on outbound HTTP calls (providers). Default false.
	InsecureSkipTLSVerify bool `yaml:"insecureSkipTLSVerify"`

	// CABundlePath is an optional path to a PEM file for custom CA
	// certificates used in outbound HTTP calls.
	CABundlePath string `yaml:"caBundlePath"`
}

// Upgrader confing struct
type Upgrader struct {
	// DisableUpdateCheck if set to true, does not check for and
	// notify about kwatch updates
	DisableUpdateCheck bool `yaml:"disableUpdateCheck"`
}

// PvcMonitor confing struct

// CrdConfig configures the KwatchConfig CRD watcher.
type CrdConfig struct {
	// Enabled if set to true, watches KwatchConfig CRs for live config changes.
	Enabled bool `yaml:"enabled"`
}

// SmartGrouping configures coalescing same-reason incidents across
// different owners into a single notification within a time window.
type SmartGrouping struct {
	// WindowSeconds is the time window in seconds for grouping same-reason
	// incidents together. Default 60. Set to 0 to disable grouping.
	WindowSeconds int `yaml:"windowSeconds"`
}

// ScheduleMonitor configures scheduling delay diagnostics.

// HealthCheck config struct
type HealthCheck struct {
	// Enabled if set to true, it will enable health check endpoint
	// By default, this value is false
	Enabled bool `yaml:"enabled"`

	// Port is the port to listen on for health check requests
	// By default, this value is 8060
	Port int `yaml:"port"`

	// Pprof if set to true, enables /debug/pprof/* profiling endpoints.
	// Disabled by default — enabling exposes runtime profiling data.
	Pprof bool `yaml:"pprof"`

	// Diagnostics if set to true, enables /incidents and /test-alert endpoints.
	// Disabled by default.
	Diagnostics bool `yaml:"diagnostics"`

	// DiagnosticsToken is an optional Bearer token required to access
	// diagnostic endpoints (/incidents, /test-alert, /deadletters).
	// When empty, diagnostic endpoints are unauthenticated.
	DiagnosticsToken string `yaml:"diagnosticsToken"`
}

// AlertRoute defines routing filters for a provider.
// An incident matching at least one route is delivered; if no routes are
// configured all incidents are delivered (current behavior).
type AlertRoute struct {
	// Namespaces is an optional list of allowed namespaces.
	Namespaces []string `yaml:"namespaces"`
	// Severities is an optional list of allowed severity levels.
	Severities []string `yaml:"severities"`
	// Reasons is an optional list of allowed reasons.
	Reasons []string `yaml:"reasons"`
}

// AuditLogConfig configures structured audit logging for all incidents.
type AuditLogConfig struct {
	// Enabled toggles audit logging. Default false.
	Enabled bool `yaml:"enabled"`
	// Output is the destination for audit log entries: "stdout" (default) or a file path.
	Output string `yaml:"output"`
}
