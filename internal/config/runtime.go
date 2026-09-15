package config

import (
	"sort"
	"time"
)

// RuntimeConfig is the immutable, derived configuration used at runtime.
// The YAML-facing Config remains the compatibility boundary; this value keeps
// normalization and compilation out of monitor construction.
type RuntimeConfig struct {
	compiled    bool
	application ApplicationRuntime
	operations  runtimeOperations
	scope       runtimeScope
	monitors    runtimeMonitors
	delivery    runtimeDelivery
	persistence runtimePersistence
	incident    runtimeIncident
}

// ApplicationRuntime contains the application settings needed after config
// loading. It deliberately has no YAML tags or nested compatibility model.
type ApplicationRuntime struct {
	ProxyURL              string
	ClusterName           string
	DisableStartupMessage bool
	LogFormatter          string
	InsecureSkipTLSVerify bool
	CABundlePath          string
}

// These private views keep the snapshot navigable without exposing mutable
// maps and slices as public fields. Accessors return defensive copies.
type runtimeOperations struct {
	telemetry   Telemetry
	upgrader    Upgrader
	healthCheck HealthCheck
	auditLog    AuditLogConfig
	resync      time.Duration
	workers     int
	watchStart  time.Time
}

type runtimeScope struct {
	allowedNamespaces   []string
	forbiddenNamespaces []string
	namespaceSelector   string
	allowedReasons      []string
	forbiddenReasons    []string
	suppression         SuppressionIndex
}

type runtimeDelivery struct {
	providerNames []string
	providers     []ProviderRuntime
	silences      []SilenceRule
	templates     map[string]string
	runbooks      map[string]string
}

type runtimePersistence struct {
	maxBaseline int
}

type runtimeIncident struct {
	config           IncidentRuntime
	severityByOwner  map[string]string
	severityByReason map[string]string
}

type runtimeMonitors struct {
	node               NodeMonitor
	nodeResource       NodeResourceMonitor
	service            ServiceMonitor
	admissionWebhook   AdmissionWebhookMonitor
	ingress            IngressMonitor
	networkPolicy      NetworkPolicyMonitor
	controlPlane       ControlPlaneMonitor
	clusterAutoscaler  ClusterAutoscalerMonitor
	job                JobMonitor
	pvc                PvcMonitor
	heartbeat          HeartbeatMonitor
	runtimeMetrics     RuntimeMetricsMonitor
	activeProbe        ActiveProbeMonitor
	kubeletTelemetry   KubeletTelemetryMonitor
	crd                CrdConfig
	clusterResource    ClusterResourceMonitor
	tls                TlsMonitor
	rollout            RolloutMonitor
	daemonSet          DaemonSetMonitor
	statefulSet        StatefulSetMonitor
	cronJob            CronJobMonitor
	hpa                HpaMonitor
	pdb                PdbMonitor
	adaptiveThresholds bool
	containerRestart   int
	schedule           ScheduleMonitor
	oom                OomMonitor
	includeEvents      bool
	includeLogs        bool
	maintenance        MaintenanceConfig
	pendingPod         PendingPodMonitor
	notReady           NotReadyMonitor
	ignoreDisruptions  bool
	maxRecentLogLines  int64
	ignoreGracefulKill bool
	reportStartup      bool
}

// CompileRuntimeConfig creates a defensive snapshot of derived settings.
func CompileRuntimeConfig(c *Config) RuntimeConfig {
	if c == nil {
		return RuntimeConfig{}
	}
	suppression := c.Suppression
	if suppressionIndexEmpty(suppression) {
		suppression = c.BuildSuppressionIndex()
	}
	providers := make([]string, 0, len(c.Alert))
	for name := range c.Alert {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return RuntimeConfig{
		compiled: true,
		application: ApplicationRuntime{
			ProxyURL:              c.App.ProxyURL,
			ClusterName:           c.App.ClusterName,
			DisableStartupMessage: c.App.DisableStartupMessage,
			LogFormatter:          c.App.LogFormatter,
			InsecureSkipTLSVerify: c.App.InsecureSkipTLSVerify,
			CABundlePath:          c.App.CABundlePath,
		},
		operations: runtimeOperations{
			telemetry:   c.Telemetry,
			upgrader:    c.Upgrader,
			healthCheck: c.HealthCheck,
			auditLog:    c.AuditLog,
			resync:      time.Duration(c.ResyncSeconds) * time.Second,
			workers:     c.Workers,
			watchStart:  c.WatchStartTime,
		},
		scope: runtimeScope{
			allowedNamespaces:   cloneStrings(c.AllowedNamespaces),
			forbiddenNamespaces: cloneStrings(c.ForbiddenNamespaces),
			namespaceSelector:   c.NamespaceSelector,
			allowedReasons:      cloneStrings(c.AllowedReasons),
			forbiddenReasons:    cloneStrings(c.ForbiddenReasons),
			suppression:         cloneSuppressionIndex(suppression),
		},
		delivery: runtimeDelivery{
			providerNames: providers,
			providers:     compileProviderRuntimes(c.Alert, providers),
			silences:      cloneSilenceRules(c.Silences),
			templates:     cloneStringMap(c.Templates),
			runbooks:      cloneStringMap(c.Runbooks),
		},
		monitors: runtimeMonitors{
			node:              c.NodeMonitor,
			nodeResource:      c.NodeResourceMonitor,
			service:           c.ServiceMonitor,
			admissionWebhook:  c.AdmissionWebhookMonitor,
			ingress:           c.IngressMonitor,
			networkPolicy:     c.NetworkPolicyMonitor,
			controlPlane:      c.ControlPlaneMonitor,
			clusterAutoscaler: c.ClusterAutoscalerMonitor,
			job:               c.JobMonitor,
			pvc:               c.PvcMonitor,
			heartbeat:         c.HeartbeatMonitor,
			runtimeMetrics:    c.RuntimeMetricsMonitor,
			activeProbe:       cloneActiveProbeMonitor(c.ActiveProbeMonitor),
			kubeletTelemetry:  c.KubeletTelemetryMonitor,
			crd: CrdConfig{
				Enabled:           c.CrdConfig.Enabled,
				FailureConditions: cloneStrings(c.CrdConfig.FailureConditions),
				GraphReferences:   cloneStrings(c.CrdConfig.GraphReferences),
			},
			clusterResource:    c.ClusterResourceMonitor,
			tls:                c.TlsMonitor,
			rollout:            c.RolloutMonitor,
			daemonSet:          c.DaemonSetMonitor,
			statefulSet:        c.StatefulSetMonitor,
			cronJob:            c.CronJobMonitor,
			hpa:                c.HpaMonitor,
			pdb:                c.PdbMonitor,
			adaptiveThresholds: c.AdaptiveThresholds,
			containerRestart:   c.ContainerRestartThreshold,
			schedule:           c.ScheduleMonitor,
			oom:                c.OomMonitor,
			includeEvents:      c.IncludeEvents == nil || *c.IncludeEvents,
			includeLogs:        c.IncludeLogs == nil || *c.IncludeLogs,
			maintenance:        c.Maintenance,
			pendingPod:         c.PendingPodMonitor,
			notReady:           c.NotReadyMonitor,
			ignoreDisruptions: c.IgnoreDisruptionTerminations == nil ||
				*c.IgnoreDisruptionTerminations,
			maxRecentLogLines:  c.MaxRecentLogLines,
			ignoreGracefulKill: c.IgnoreFailedGracefulShutdown,
			reportStartup:      c.ReportStartupBaseline,
		},
		persistence: runtimePersistence{
			maxBaseline: c.Correlation.MaxBaseline,
		},
		incident: runtimeIncident{
			config: IncidentRuntime{
				Window: c.Correlation.Window.Duration(),
				LifecycleInterval: time.Duration(
					c.Correlation.LifecycleInterval,
				) * time.Minute,
				EscalationEnabled:         c.Correlation.Escalation.Enabled,
				EscalationTiers:           cloneInts(c.Correlation.Escalation.Tiers),
				InhibitNodeSuppressesPods: c.Inhibition.NodeSuppressesPods,
				RenotifyIntervalBySeverity: compileRenotify(
					c.Correlation.Renotify.IntervalBySeverity,
				),
				RenotifyMaxPerIncident:   c.Correlation.Renotify.MaxPerIncident,
				ResolveHoldDown:          c.Correlation.ResolveHoldDown.Duration(),
				SmartGroupingWindow:      c.SmartGrouping.WindowSeconds.Duration(),
				NamespaceFanOutThreshold: c.SmartGrouping.NamespaceFanOutThreshold,
				MaxBaseline:              c.Correlation.MaxBaseline,
			},
			severityByOwner:  cloneStringMap(c.SeverityByOwnerKind),
			severityByReason: cloneStringMap(c.SeverityByReason),
		},
	}
}

func suppressionIndexEmpty(index SuppressionIndex) bool {
	return len(index.ContainerNames) == 0 &&
		len(index.PodNamePatterns) == 0 &&
		len(index.LogPatterns) == 0 &&
		len(index.ContainerMessages) == 0 &&
		len(index.EventMessages) == 0 &&
		len(index.NodeReasons) == 0 &&
		len(index.NodeMessages) == 0
}

// IncidentRuntime contains normalized incident lifecycle settings. It keeps
// the state machine independent from the YAML-facing configuration model.
type IncidentRuntime struct {
	Window                     time.Duration
	LifecycleInterval          time.Duration
	EscalationEnabled          bool
	EscalationTiers            []int
	InhibitNodeSuppressesPods  bool
	RenotifyIntervalBySeverity map[string]time.Duration
	RenotifyMaxPerIncident     int
	ResolveHoldDown            time.Duration
	SmartGroupingWindow        time.Duration
	NamespaceFanOutThreshold   int
	MaxBaseline                int
}

func cloneInts(values []int) []int { return append([]int(nil), values...) }

func compileRenotify(values map[string]int) map[string]time.Duration {
	result := make(map[string]time.Duration, len(values))
	for key, value := range values {
		result[key] = time.Duration(value) * time.Minute
	}
	return result
}
