package config

import (
	"regexp"
	"time"
)

// RuntimeConfigFor returns the derived snapshot used by runtime components.
// Loaded configurations already carry it; direct callers are compiled without
// mutating the YAML-facing configuration.
func RuntimeConfigFor(c *Config) RuntimeConfig {
	if c == nil {
		return RuntimeConfig{}
	}
	if c.Runtime.Compiled() {
		return c.Runtime
	}
	return CompileRuntimeConfig(c)
}

// Compiled reports whether this snapshot was produced from a configuration.
// It lets package-level tests that build Config values directly retain the
// same defaults as the file loader without making runtime consumers read raw
// configuration fields.
func (r RuntimeConfig) Compiled() bool {
	return r.compiled
}

// Application returns the normalized application identity and outbound
// settings. The value contains no mutable reference fields.
func (r RuntimeConfig) Application() ApplicationRuntime {
	return r.application
}

// DisableStartupMessage reports whether startup notifications are disabled.
// Keeping this as a scalar accessor prevents startup from retaining the raw
// application configuration value.
func (r RuntimeConfig) DisableStartupMessage() bool {
	return r.application.DisableStartupMessage
}

// WithApplicationIdentity returns a copy with the compatibility application
// identity replaced. It is used only by legacy delivery initialization.
func WithApplicationIdentity(
	r RuntimeConfig,
	clusterName string,
) RuntimeConfig {
	r.application.ClusterName = clusterName
	return r
}

// Telemetry returns the normalized adoption telemetry policy.
func (r RuntimeConfig) Telemetry() Telemetry { return r.operations.telemetry }

// Upgrader returns the normalized update-check policy.
func (r RuntimeConfig) Upgrader() Upgrader { return r.operations.upgrader }

// HealthCheck returns the normalized health-server policy.
func (r RuntimeConfig) HealthCheck() HealthCheck {
	return r.operations.healthCheck
}

// AuditLog returns the normalized audit logging policy.
func (r RuntimeConfig) AuditLog() AuditLogConfig {
	return r.operations.auditLog
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneSuppressionIndex(index SuppressionIndex) SuppressionIndex {
	return SuppressionIndex{
		ContainerNames:    cloneStrings(index.ContainerNames),
		PodNamePatterns:   append([]*regexp.Regexp(nil), index.PodNamePatterns...),
		LogPatterns:       append([]*regexp.Regexp(nil), index.LogPatterns...),
		ContainerMessages: cloneStrings(index.ContainerMessages),
		EventMessages:     cloneStrings(index.EventMessages),
		NodeReasons:       cloneStrings(index.NodeReasons),
		NodeMessages:      cloneStrings(index.NodeMessages),
	}
}

// The accessors return copies so callers cannot mutate the compiled snapshot.
func (r RuntimeConfig) AllowedNamespaces() []string {
	return cloneStrings(r.scope.allowedNamespaces)
}

func (r RuntimeConfig) ForbiddenNamespaces() []string {
	return cloneStrings(r.scope.forbiddenNamespaces)
}

// NamespaceSelector returns the label selector used for dynamic namespace
// scope discovery.
func (r RuntimeConfig) NamespaceSelector() string {
	return r.scope.namespaceSelector
}

func (r RuntimeConfig) AllowedReasons() []string {
	return cloneStrings(r.scope.allowedReasons)
}

func (r RuntimeConfig) ForbiddenReasons() []string {
	return cloneStrings(r.scope.forbiddenReasons)
}

func (r RuntimeConfig) ProviderNames() []string {
	return cloneStrings(r.delivery.providerNames)
}

// Providers returns a defensive copy of normalized provider configuration.
func (r RuntimeConfig) Providers() []ProviderRuntime {
	return cloneProviderRuntimes(r.delivery.providers)
}

// NodeMonitor returns the normalized Node policy configuration.
func (r RuntimeConfig) NodeMonitor() NodeMonitor { return r.monitors.node }

// NodeResourceMonitor returns normalized node resource policy.
func (r RuntimeConfig) NodeResourceMonitor() NodeResourceMonitor {
	return r.monitors.nodeResource
}

func (r RuntimeConfig) ServiceMonitor() ServiceMonitor {
	return r.monitors.service
}

func (r RuntimeConfig) AdmissionWebhookMonitor() AdmissionWebhookMonitor {
	return r.monitors.admissionWebhook
}

func (r RuntimeConfig) IngressMonitor() IngressMonitor {
	return r.monitors.ingress
}

func (r RuntimeConfig) NetworkPolicyMonitor() NetworkPolicyMonitor {
	return r.monitors.networkPolicy
}

func (r RuntimeConfig) ControlPlaneMonitor() ControlPlaneMonitor {
	return r.monitors.controlPlane
}

func (r RuntimeConfig) ClusterAutoscalerMonitor() ClusterAutoscalerMonitor {
	return r.monitors.clusterAutoscaler
}

func (r RuntimeConfig) JobMonitor() JobMonitor { return r.monitors.job }

// PvcMonitor returns normalized PVC usage policy.
func (r RuntimeConfig) PvcMonitor() PvcMonitor { return r.monitors.pvc }

// HeartbeatMonitor returns the external dead-man's-switch policy.
func (r RuntimeConfig) HeartbeatMonitor() HeartbeatMonitor {
	return r.monitors.heartbeat
}

// RuntimeMetricsMonitor returns the optional metrics-server policy.
func (r RuntimeConfig) RuntimeMetricsMonitor() RuntimeMetricsMonitor {
	return r.monitors.runtimeMetrics
}

// ActiveProbeMonitor returns a defensive copy of active probe settings.
func (r RuntimeConfig) ActiveProbeMonitor() ActiveProbeMonitor {
	return cloneActiveProbeMonitor(r.monitors.activeProbe)
}

// KubeletTelemetryMonitor returns normalized kubelet telemetry policy.
func (r RuntimeConfig) KubeletTelemetryMonitor() KubeletTelemetryMonitor {
	return r.monitors.kubeletTelemetry
}

// CrdConfig returns a defensive copy of dynamic status watcher settings.
func (r RuntimeConfig) CrdConfig() CrdConfig {
	return CrdConfig{
		Enabled:           r.monitors.crd.Enabled,
		FailureConditions: cloneStrings(r.monitors.crd.FailureConditions),
		GraphReferences:   cloneStrings(r.monitors.crd.GraphReferences),
	}
}

// Silences returns the delivery suppression rules as a defensive copy.
func (r RuntimeConfig) Silences() []SilenceRule {
	return cloneSilenceRules(r.delivery.silences)
}

// Templates returns a defensive copy of configured incident templates.
func (r RuntimeConfig) Templates() map[string]string {
	return cloneStringMap(r.delivery.templates)
}

// Runbooks returns a defensive copy of configured runbook links.
func (r RuntimeConfig) Runbooks() map[string]string {
	return cloneStringMap(r.delivery.runbooks)
}

// MaxBaseline returns the configured cap for startup baseline entries.
func (r RuntimeConfig) MaxBaseline() int {
	return r.persistence.maxBaseline
}

// ClusterResourceMonitor returns the normalized cluster-resource policy.
func (r RuntimeConfig) ClusterResourceMonitor() ClusterResourceMonitor {
	return r.monitors.clusterResource
}

// TlsMonitor returns the normalized TLS policy configuration.
func (r RuntimeConfig) TlsMonitor() TlsMonitor { return r.monitors.tls }

// RolloutMonitor returns Deployment rollout policy.
func (r RuntimeConfig) RolloutMonitor() RolloutMonitor {
	return r.monitors.rollout
}

// DaemonSetMonitor returns DaemonSet rollout policy.
func (r RuntimeConfig) DaemonSetMonitor() DaemonSetMonitor {
	return r.monitors.daemonSet
}

// StatefulSetMonitor returns StatefulSet rollout policy.
func (r RuntimeConfig) StatefulSetMonitor() StatefulSetMonitor {
	return r.monitors.statefulSet
}

// CronJobMonitor returns CronJob policy.
func (r RuntimeConfig) CronJobMonitor() CronJobMonitor {
	return r.monitors.cronJob
}

// HpaMonitor returns HPA policy.
func (r RuntimeConfig) HpaMonitor() HpaMonitor { return r.monitors.hpa }

// PdbMonitor returns PDB policy.
func (r RuntimeConfig) PdbMonitor() PdbMonitor { return r.monitors.pdb }

// AdaptiveThresholds reports whether workload grace is enabled.
func (r RuntimeConfig) AdaptiveThresholds() bool {
	return r.monitors.adaptiveThresholds
}

// ContainerRestartThreshold returns the normalized restart threshold.
func (r RuntimeConfig) ContainerRestartThreshold() int {
	return r.monitors.containerRestart
}

// ScheduleMonitor returns scheduling-delay policy.
func (r RuntimeConfig) ScheduleMonitor() ScheduleMonitor {
	return r.monitors.schedule
}

// OomMonitor returns OOM detection policy.
func (r RuntimeConfig) OomMonitor() OomMonitor { return r.monitors.oom }

func (r RuntimeConfig) SuppressionIndex() SuppressionIndex {
	return cloneSuppressionIndex(r.scope.suppression)
}

func (r RuntimeConfig) ResyncInterval() time.Duration {
	return r.operations.resync
}

func (r RuntimeConfig) Workers() int {
	return r.operations.workers
}

// IncludeEvents reports whether event evidence should be included in output.
func (r RuntimeConfig) IncludeEvents() bool { return r.monitors.includeEvents }

// IncludeLogs reports whether log evidence should be included in output.
func (r RuntimeConfig) IncludeLogs() bool { return r.monitors.includeLogs }

// Maintenance returns the immutable maintenance annotation policy.
func (r RuntimeConfig) Maintenance() MaintenanceConfig {
	return r.monitors.maintenance
}

// PendingPodMonitor returns normalized pending-Pod policy.
func (r RuntimeConfig) PendingPodMonitor() PendingPodMonitor {
	return r.monitors.pendingPod
}

// NotReadyMonitor returns normalized not-ready Pod policy.
func (r RuntimeConfig) NotReadyMonitor() NotReadyMonitor {
	return r.monitors.notReady
}

// IgnoreDisruptionTerminations reports whether disruption terminations are
// excluded from container findings.
func (r RuntimeConfig) IgnoreDisruptionTerminations() bool {
	return r.monitors.ignoreDisruptions
}

// MaxRecentLogLines returns the configured log tail limit.
func (r RuntimeConfig) MaxRecentLogLines() int64 {
	return r.monitors.maxRecentLogLines
}

// IgnoreFailedGracefulShutdown reports whether expected forced termination is
// excluded from container findings.
func (r RuntimeConfig) IgnoreFailedGracefulShutdown() bool {
	return r.monitors.ignoreGracefulKill
}

// ReportStartupBaseline reports whether the application should send the
// startup baseline summary.
func (r RuntimeConfig) ReportStartupBaseline() bool {
	return r.monitors.reportStartup
}

// Incident returns a defensive copy of normalized lifecycle settings.
func (r RuntimeConfig) Incident() IncidentRuntime {
	result := r.incident.config
	result.EscalationTiers = cloneInts(result.EscalationTiers)
	result.RenotifyIntervalBySeverity = cloneDurationMap(
		result.RenotifyIntervalBySeverity,
	)
	return result
}

func cloneDurationMap(
	values map[string]time.Duration,
) map[string]time.Duration {
	if values == nil {
		return nil
	}
	result := make(map[string]time.Duration, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (r RuntimeConfig) SeverityByOwnerKind() map[string]string {
	return cloneStringMap(r.incident.severityByOwner)
}

func (r RuntimeConfig) SeverityByReason() map[string]string {
	return cloneStringMap(r.incident.severityByReason)
}

// WatchStartTime is the application start boundary used by age-based policy.
func (r RuntimeConfig) WatchStartTime() time.Time {
	return r.operations.watchStart
}

func cloneActiveProbeMonitor(m ActiveProbeMonitor) ActiveProbeMonitor {
	m.HTTP = append([]HTTPProbeTarget(nil), m.HTTP...)
	m.TCP = append([]TCPProbeTarget(nil), m.TCP...)
	m.DNS = append([]DNSProbeTarget(nil), m.DNS...)
	m.ExcludeNamespaces = cloneStrings(m.ExcludeNamespaces)
	return m
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneSilenceRules(rules []SilenceRule) []SilenceRule {
	if len(rules) == 0 {
		return nil
	}
	result := make([]SilenceRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, SilenceRule{
			Namespaces:        cloneStrings(rule.Namespaces),
			Reasons:           cloneStrings(rule.Reasons),
			PodNamePatterns:   cloneStrings(rule.PodNamePatterns),
			ContainerNames:    cloneStrings(rule.ContainerNames),
			LogPatterns:       cloneStrings(rule.LogPatterns),
			ContainerMessages: cloneStrings(rule.ContainerMessages),
			EventMessages:     cloneStrings(rule.EventMessages),
			NodeReasons:       cloneStrings(rule.NodeReasons),
			NodeMessages:      cloneStrings(rule.NodeMessages),
		})
	}
	return result
}
