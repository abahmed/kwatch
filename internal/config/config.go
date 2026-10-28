package config

// Config is the YAML-facing kwatch configuration.
type Config struct {
	// App general configuration
	App App `yaml:"app"`

	// Telemetry configures the minimal adoption heartbeat.
	Telemetry Telemetry `yaml:"telemetry"`

	// License holds the kwatch Pro license key.
	License License `yaml:"license"`

	// ClusterCheckins is deprecated and ignored: kwatch no longer sends
	// cluster check-ins. It is still accepted so an older config loads,
	// and Warnings asks for its removal.
	ClusterCheckins *bool `yaml:"clusterCheckins,omitempty"`

	// Upgrader configuration
	Upgrader Upgrader `yaml:"upgrader"`

	// HeartbeatMonitor pings an external dead man's switch.
	HeartbeatMonitor HeartbeatMonitor `yaml:"heartbeatMonitor"`

	// HealthCheck configuration
	HealthCheck HealthCheck `yaml:"healthCheck"`

	// Dashboard configures the read-only incident dashboard API. Pro only,
	// off by default.
	Dashboard Dashboard `yaml:"dashboard"`

	// Maintenance names the annotations that mark deliberate maintenance.
	// Incidents whose objects are under maintenance are not delivered.
	Maintenance MaintenanceConfig `yaml:"maintenance"`

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

	// NamespaceSelector is a Kubernetes label selector for namespaces in
	// scope. Mutually exclusive with Namespaces.
	NamespaceSelector string `yaml:"namespaceSelector"`

	// IgnoreContainerNames optional list of container names to ignore
	IgnoreContainerNames []string `yaml:"ignoreContainerNames"`

	// IgnorePodNames optional list of pod name regexp patterns to ignore
	IgnorePodNames []string `yaml:"ignorePodNames"`

	// IgnoreContainerMessages optional list of substrings; a container
	// finding whose message contains any entry is silenced.
	IgnoreContainerMessages []string `yaml:"ignoreContainerMessages"`

	// IgnoreNodeReasons is an optional list of node reasons to silence.
	IgnoreNodeReasons []string `yaml:"ignoreNodeReasons"`
	// IgnoreNodeMessages is an optional list of node messages to silence.
	IgnoreNodeMessages []string `yaml:"ignoreNodeMessages"`

	// Silences is an optional list of silence rules.
	Silences []SilenceRule `yaml:"silences"`

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

	// ResyncSeconds is the interval (in seconds) for periodic informer
	// resyncs, a safety net for missed watch events. Zero disables it.
	ResyncSeconds int `yaml:"resyncSeconds"`

	// SeverityByOwnerKind raises or lowers incidents by the kind of the
	// affected workload, e.g. {"StatefulSet": "critical"}.
	SeverityByOwnerKind map[string]string `yaml:"severityByOwnerKind"`

	// SeverityByReason overrides severity by reason and is checked before
	// the owner kind, e.g. {"OOMKilled": "critical"}.
	SeverityByReason map[string]string `yaml:"severityByReason"`

	// ActiveProbeMonitor performs explicitly configured HTTP, TCP, and DNS
	// checks. It is opt-in because Kubernetes cannot infer safe probe targets.
	ActiveProbeMonitor ActiveProbeMonitor `yaml:"activeProbeMonitor"`

	// CrdConfig configures the KwatchConfig CRD watcher.
	CrdConfig CrdConfig `yaml:"crd"`

	// Templates maps a reason (lowercased) to a Go text/template for plain
	// text providers. The template receives .Message and .Text.
	Templates map[string]string `yaml:"templates"`

	// Runbooks maps reasons to documentation URLs added to the steps of a
	// incident with that reason.
	Runbooks map[string]string `yaml:"runbooks"`

	// AuditLog configures the JSON decision log.
	AuditLog AuditLogConfig `yaml:"auditLog"`

	// Kubelet configures the direct connection to each node's kubelet.
	Kubelet KubeletConfig `yaml:"kubelet"`

	// Watch turns optional watched resources on or off.
	Watch WatchConfig `yaml:"watch"`

	// Runtime is the defensive snapshot of derived configuration used by
	// composition and runtime components. It is never decoded from YAML.
	Runtime RuntimeConfig `yaml:"-"`
	// syntheticSilences counts trailing rules generated from legacy ignore*
	// fields. It is derived state and is never serialized.
	syntheticSilences int
	// unknownKeys are config keys the file sets that no field reads, such
	// as removed options or typos. They are logged, never fatal.
	unknownKeys []string
	// envWarnings are the deployment environment values that were ignored
	// (an unparsable boolean). Derived state, never serialized.
	envWarnings []string
	// fileWarnings are findings about how the file was written, such as a
	// literal license key. They are known only before expansion.
	fileWarnings []string
}

// Telemetry configures the minimal adoption heartbeat. It is enabled
// for official builds by default and can be disabled by the operator.
// Telemetry is the weekly adoption heartbeat: a per-cluster UUID and the
// kwatch version, nothing else. On by default, and announced at startup with
// the endpoint and effective setting without exposing the payload.
type Telemetry struct {
	Enabled bool `yaml:"enabled"`
}

// License configures the kwatch Pro license. The key is a credential:
// reference it with ${ENV} or ${file:/path}, or set KWATCH_LICENSE_KEY,
// which wins over this file. It is never logged.
type License struct {
	// Key is the license key. Empty runs the Community edition.
	Key string `yaml:"key"`
	// Endpoint is the license server. It exists for testing only.
	Endpoint string `yaml:"endpoint"`
}

// DefaultLicenseEndpoint is the license server. It matches
// license.DefaultEndpoint (a test in internal/app keeps them equal).
const DefaultLicenseEndpoint = "https://api.kwatch.dev"

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

	// HideProHints removes the one-line Pro note or teaser from
	// Community messages and the startup count of Pro findings. /status
	// still counts them. Default false.
	HideProHints bool `yaml:"hideProHints"`

	// CommunityProvider names the alert provider the Community edition
	// keeps when more than one is configured (it runs one). Empty keeps
	// the first by name; a name that is not configured is warned about
	// and falls back the same way. Pro runs every provider.
	CommunityProvider string `yaml:"communityProvider"`
}

// Upgrader confing struct
type Upgrader struct {
	// DisableUpdateCheck if set to true, does not check for and
	// notify about kwatch updates
	DisableUpdateCheck bool `yaml:"disableUpdateCheck"`
}

// CrdConfig configures the KwatchConfig CRD watcher.
type CrdConfig struct {
	// Enabled if set to true, watches KwatchConfig CRs for live config changes.
	Enabled bool `yaml:"enabled"`
}

// HealthCheck config struct
type HealthCheck struct {
	// Enabled if set to true, it will enable health check endpoint
	// By default, this value is true.
	Enabled bool `yaml:"enabled"`

	// Port is the port to listen on for health check requests
	// By default, this value is 8060
	Port int `yaml:"port"`

	// StatusEndpoint if set to true, serves the read-only /status
	// diagnostics view (open incidents with object names) on the health
	// port, without authentication. By default, this value is false.
	StatusEndpoint bool `yaml:"statusEndpoint"`
}

// Dashboard configures the read-only incident dashboard API. It is Pro
// only and off by default. The dashboard runs on its own listener, never
// on the health port.
type Dashboard struct {
	// Enabled serves the read-only dashboard API on a separate listener.
	// By default, this value is false.
	Enabled bool `yaml:"enabled"`

	// Port is the port to listen on for dashboard requests.
	// By default, this value is 8061.
	Port int `yaml:"port"`

	// Mode is how people reach the dashboard: "local" through a
	// port-forward, or "shared" through an Ingress with TLS. Shared mode
	// marks the session cookie Secure and refuses sign-in over plain
	// HTTP. By default, this value is "local".
	Mode string `yaml:"mode"`
}

// Dashboard modes.
const (
	DashboardLocal  = "local"
	DashboardShared = "shared"
)

// AlertRoute defines routing filters for a provider. An incident matching at
// least one route is delivered; without routes everything is delivered.
type AlertRoute struct {
	// Namespaces is an optional list of allowed namespaces.
	Namespaces []string `yaml:"namespaces"`
	// Severities is an optional list of allowed severity levels.
	Severities []string `yaml:"severities"`
	// Reasons is an optional list of allowed reasons.
	Reasons []string `yaml:"reasons"`
	// Owners is an optional list of allowed owners: the kwatch.dev/owner
	// label or annotation of the failing workload, else of its
	// namespace. An incident with no owner matches no route that sets
	// Owners.
	Owners []string `yaml:"owners"`
}

// AuditLogConfig configures the JSON decision log.
type AuditLogConfig struct {
	// Enabled toggles audit logging. Default true; entries go to stdout unless
	// an output file is configured.
	Enabled bool `yaml:"enabled"`
	// Output is the destination for audit log entries: "stdout" (default) or a
	// file path.
	Output string `yaml:"output"`
}

// KubeletConfig configures how kwatch reads node stats and metrics
// directly from each kubelet on port 10250.
type KubeletConfig struct {
	// InsecureSkipVerify skips verification of the kubelet serving
	// certificate. Leave it false when kubelet serving certificates are
	// signed by the cluster CA; set it only for clusters whose kubelets
	// use self-signed certificates. Default false.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

// WatchConfig turns optional watched resources on or off.
type WatchConfig struct {
	// Secrets watches Secrets (values are hashed, never stored). When
	// false kwatch needs no Secret RBAC and checks that need Secrets
	// report that they cannot verify. Default true.
	Secrets bool `yaml:"secrets"`
}
