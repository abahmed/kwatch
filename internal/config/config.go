package config

import (
	"regexp"
	"time"

	"k8s.io/klog/v2"
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

	// PendingPodMonitor configures Pending-phase pod detection.
	PendingPodMonitor PendingPodMonitor `yaml:"pendingPodMonitor"`

	// RolloutMonitor configures stuck-rollout detection for Deployments.
	RolloutMonitor RolloutMonitor `yaml:"rolloutMonitor"`

	// JobMonitor configures failed/suspended Job detection.
	JobMonitor JobMonitor `yaml:"jobMonitor"`

	// DaemonSetMonitor configures rollout-stuck detection for DaemonSets.
	DaemonSetMonitor DaemonSetMonitor `yaml:"daemonSetMonitor"`

	// CronJobMonitor configures failed/suspended CronJob detection.
	CronJobMonitor CronJobMonitor `yaml:"cronJobMonitor"`

	// HpaMonitor configures HPA-maxed-out detection.
	HpaMonitor HpaMonitor `yaml:"hpaMonitor"`

	// TlsMonitor configures TLS certificate expiry monitoring.
	TlsMonitor TlsMonitor `yaml:"tlsMonitor"`

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

	// StormConfig configures digest aggregation for alert storms.
	StormConfig StormConfig `yaml:"storm"`

	// CrdConfig configures the KwatchConfig CRD watcher.
	CrdConfig CrdConfig `yaml:"crd"`

	// Templates maps incident reason (lowercased) to Go text/template string.
	// Available template keys: {{.Incident.Key}}, {{.Incident.Reason}},
	// {{.Action}}, {{.Message}}. Missing keys render as empty string.
	Templates map[string]string `yaml:"templates"`

	// Runbooks maps Kubernetes event reasons to documentation URLs.
	// When a reason matches, the URL is appended to the incident hint.
	Runbooks map[string]string `yaml:"runbooks"`

	// LLM configures the self-hosted AI enrichment sidecar.
	LLM LLMConfig `yaml:"llm"`

	// DashboardURLTemplate is an optional URL template with {namespace}/{owner}/{pod}
	// placeholders, rendered in alerts as a deep-link to a dashboard.
	DashboardURLTemplate string `yaml:"dashboardURLTemplate"`
}

// LLMConfig controls the optional AI enrichment sidecar.
// When enabled, a kwatch-llm sidecar is deployed alongside kwatch in the pod.
// The model (kwatch-triage), endpoint (localhost:8080), redaction, and timeouts
// are baked into the sidecar image and code constants — no other knobs.
type LLMConfig struct {
	// Enabled toggles the AI enrichment feature. Default false.
	// When true, the kwatch-llm sidecar container is rendered in the pod spec
	// and kwatch enriches incidents with AI root-cause analysis.
	Enabled bool `yaml:"enabled"`
}

// KnownProviders is the canonical set of known alert provider names.
// Both alert.Init and config validation reference this to prevent drift.
var KnownProviders = map[string]bool{
	"slack": true, "pagerduty": true, "discord": true, "telegram": true,
	"teams": true, "email": true, "rocketchat": true, "mattermost": true,
	"opsgenie": true, "matrix": true, "dingtalk": true, "feishu": true,
	"webhook": true, "zenduty": true, "googlechat": true,
}

// StormConfig configures digest aggregation for high-frequency incidents.
type StormConfig struct {
	// Enabled if set to true, excessive creates are batched into a digest.
	Enabled bool `yaml:"enabled"`
	// Threshold is the max creates per window before digest mode activates.
	Threshold int `yaml:"threshold"`
	// WindowMinutes is the sliding window for tracking create rate.
	WindowMinutes int `yaml:"windowMinutes"`
	// DigestIntervalMinutes is how often a digest summary is sent.
	DigestIntervalMinutes int `yaml:"digestIntervalMinutes"`
}

// Inhibition configures cross-monitor suppression rules.
type Inhibition struct {
	// NodeSuppressesPods if true, pod incidents on a node with an active
	// node incident are suppressed to reduce noise. Default true.
	NodeSuppressesPods bool `yaml:"nodeSuppressesPods"`
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
}

// JobMonitor config struct
type JobMonitor struct {
	// Enabled if set to true, it will watch Jobs for failures
	// By default, this value is true
	Enabled bool `yaml:"enabled"`
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
}

// CrdConfig configures the KwatchConfig CRD watcher.
type CrdConfig struct {
	// Enabled if set to true, watches KwatchConfig CRs for live config changes.
	Enabled bool `yaml:"enabled"`
}

// PendingPodMonitor config struct
type PendingPodMonitor struct {
	// Enabled if set to true, it will watch pods stuck in Pending phase
	Enabled bool `yaml:"enabled"`

	// Threshold is the duration (in seconds) a pod can remain
	// in Pending phase before an alert is raised. Default 300 (5 min).
	Threshold int `yaml:"threshold"`
}

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

// SilenceRule defines an alert suppression rule.
// An incident matching any silence rule is suppressed entirely.
type SilenceRule struct {
	// Namespaces is an optional list of namespaces to silence.
	Namespaces []string `yaml:"namespaces"`
	// Reasons is an optional list of reasons to silence.
	Reasons []string `yaml:"reasons"`
	// PodNamePatterns is an optional list of regex patterns for pod names to silence.
	PodNamePatterns []string `yaml:"podNamePatterns"`
	// ContainerNames is an optional list of container names to silence.
	ContainerNames []string `yaml:"containerNames"`
	// LogPatterns is an optional list of regex patterns for log content to silence.
	LogPatterns []string `yaml:"logPatterns"`
	// ContainerMessages is an optional list of substrings; if a container
	// status message contains any entry, the incident is suppressed.
	ContainerMessages []string `yaml:"containerMessages"`
	// NodeReasons is an optional list of node reasons to silence.
	NodeReasons []string `yaml:"nodeReasons"`
	// NodeMessages is an optional list of substrings; if a node condition
	// message contains any entry, the incident is suppressed.
	NodeMessages []string `yaml:"nodeMessages"`
}

// SuppressionIndex is a flat compiled view of all suppression rules (both from
// explicit Silences and deprecated ignore* fields) for efficient detect-time
// filtering.
type SuppressionIndex struct {
	ContainerNames    []string
	PodNamePatterns   []*regexp.Regexp
	LogPatterns       []*regexp.Regexp
	ContainerMessages []string
	NodeReasons       []string
	NodeMessages      []string
}

// BuildSuppressionIndex merges deprecated ignore* fields with explicit
// SilenceRules and returns a flat SuppressionIndex for detect-time filters.
func (c *Config) BuildSuppressionIndex() SuppressionIndex {
	idx := SuppressionIndex{}
	seenContainer := map[string]bool{}
	seenPodPat := map[string]bool{}
	seenLogPat := map[string]bool{}
	seenMsg := map[string]bool{}
	seenNodeReasons := map[string]bool{}
	seenNodeMsg := map[string]bool{}

	add := func(sr SilenceRule) {
		for _, n := range sr.ContainerNames {
			if !seenContainer[n] {
				idx.ContainerNames = append(idx.ContainerNames, n)
				seenContainer[n] = true
			}
		}
		for _, p := range sr.PodNamePatterns {
			if !seenPodPat[p] {
				if re, err := regexp.Compile(p); err == nil {
					idx.PodNamePatterns = append(idx.PodNamePatterns, re)
					seenPodPat[p] = true
				} else {
					klog.ErrorS(err, "invalid suppression pod name pattern", "pattern", p)
				}
			}
		}
		for _, p := range sr.LogPatterns {
			if !seenLogPat[p] {
				if re, err := regexp.Compile(p); err == nil {
					idx.LogPatterns = append(idx.LogPatterns, re)
					seenLogPat[p] = true
				} else {
					klog.ErrorS(err, "invalid suppression log pattern", "pattern", p)
				}
			}
		}
		for _, m := range sr.ContainerMessages {
			if !seenMsg[m] {
				idx.ContainerMessages = append(idx.ContainerMessages, m)
				seenMsg[m] = true
			}
		}
		for _, r := range sr.NodeReasons {
			if !seenNodeReasons[r] {
				idx.NodeReasons = append(idx.NodeReasons, r)
				seenNodeReasons[r] = true
			}
		}
		for _, m := range sr.NodeMessages {
			if !seenNodeMsg[m] {
				idx.NodeMessages = append(idx.NodeMessages, m)
				seenNodeMsg[m] = true
			}
		}
	}

	for _, sr := range c.Silences {
		add(sr)
	}
	// Also include deprecated ignore* fields directly (they may also appear as
	// synthetic SilenceRules, but this ensures they're present regardless).
	for _, n := range c.IgnoreContainerNames {
		if !seenContainer[n] {
			idx.ContainerNames = append(idx.ContainerNames, n)
			seenContainer[n] = true
		}
	}
	// Compile these if not already covered by silences
	for _, p := range c.IgnorePodNames {
		if !seenPodPat[p] {
			if re, err := regexp.Compile(p); err == nil {
				idx.PodNamePatterns = append(idx.PodNamePatterns, re)
				seenPodPat[p] = true
			} else {
				klog.ErrorS(err, "invalid ignorePodName pattern", "pattern", p)
			}
		}
	}
	for _, p := range c.IgnoreLogPatterns {
		if !seenLogPat[p] {
			if re, err := regexp.Compile(p); err == nil {
				idx.LogPatterns = append(idx.LogPatterns, re)
				seenLogPat[p] = true
			} else {
				klog.ErrorS(err, "invalid ignoreLogPattern", "pattern", p)
			}
		}
	}
	for _, m := range c.IgnoreContainerMessages {
		if !seenMsg[m] {
			idx.ContainerMessages = append(idx.ContainerMessages, m)
			seenMsg[m] = true
		}
	}
	for _, r := range c.IgnoreNodeReasons {
		if !seenNodeReasons[r] {
			idx.NodeReasons = append(idx.NodeReasons, r)
			seenNodeReasons[r] = true
		}
	}
	for _, m := range c.IgnoreNodeMessages {
		if !seenNodeMsg[m] {
			idx.NodeMessages = append(idx.NodeMessages, m)
			seenNodeMsg[m] = true
		}
	}
	return idx
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

// Correlation config struct
type Correlation struct {
	// Window is the time window (in minutes) for correlating events.
	// Events outside this window start a new incident.
	Window int `yaml:"window"`

	// LifecycleInterval is the interval (in minutes) for checking
	// lifecycle transitions (stale, resolved). Default 1.
	LifecycleInterval int `yaml:"lifecycleInterval"`

	// ResolveHoldDown is the seconds to wait after a condition clears before
	// emitting "resolved". If it recurs within this window the incident stays
	// open (flap dampening). Default 0 = resolve immediately.
	ResolveHoldDown int `yaml:"resolveHoldDown"`

	// Escalation configures restart-count-based severity escalation.
	Escalation EscalationConfig `yaml:"escalation"`

	// Renotify configures periodic re-notification via intervalBySeverity["default"].
	Renotify RenotifyConfig `yaml:"renotify"`

	// MaxBaseline is the maximum number of baseline entries to keep.
	// Default 2000.
	MaxBaseline int `yaml:"maxBaseline"`
}

// RenotifyConfig configures periodic re-notification for active incidents.
type RenotifyConfig struct {
	// IntervalBySeverity is the minimum time (in minutes) between renotifications,
	// keyed by severity ("normal", "high", "critical"). Use "default" key as
	// fallback when a severity has no entry. 0 disables renotify.
	IntervalBySeverity map[string]int `yaml:"intervalBySeverity"`
	// MaxPerIncident is the maximum number of renotifications per incident. Default 3.
	MaxPerIncident int `yaml:"maxPerIncident"`
}

// EscalationConfig configures severity escalation when restart count
// crosses configured thresholds.
type EscalationConfig struct {
	// Enabled if set to true, severity escalates when restart count
	// crosses configured tier boundaries.
	Enabled bool `yaml:"enabled"`

	// Tiers is an ordered list of restart count thresholds. When the
	// RestartCount crosses a tier, severity escalates one level.
	// Example: [3, 10, 50] → at 3+ restarts → "high", 10+ → "critical".
	Tiers []int `yaml:"tiers"`
}
