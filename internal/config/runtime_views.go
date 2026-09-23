package config

import "time"

// ScopeRuntime groups the normalized namespace, reason, and suppression
// policy. It is returned by value so callers cannot replace the snapshot's
// policy maps or slices.
type ScopeRuntime struct {
	values runtimeScope
}

func (s ScopeRuntime) AllowedNamespaces() []string {
	return cloneStrings(s.values.allowedNamespaces)
}

func (s ScopeRuntime) ForbiddenNamespaces() []string {
	return cloneStrings(s.values.forbiddenNamespaces)
}

func (s ScopeRuntime) NamespaceSelector() string {
	return s.values.namespaceSelector
}

func (s ScopeRuntime) AllowedReasons() []string {
	return cloneStrings(s.values.allowedReasons)
}

func (s ScopeRuntime) ForbiddenReasons() []string {
	return cloneStrings(s.values.forbiddenReasons)
}

func (s ScopeRuntime) SuppressionIndex() SuppressionIndex {
	return cloneSuppressionIndex(s.values.suppression)
}

// MonitorRuntime groups all normalized monitor policies. The individual
// policy methods keep resource ownership visible when a contributor adds a
// monitor without exposing the internal aggregate structure.
type MonitorRuntime struct {
	values runtimeMonitors
}

func (m MonitorRuntime) Node() NodeMonitor { return m.values.node }

func (m MonitorRuntime) NodeResource() NodeResourceMonitor {
	return m.values.nodeResource
}

func (m MonitorRuntime) Service() ServiceMonitor { return m.values.service }

func (m MonitorRuntime) AdmissionWebhook() AdmissionWebhookMonitor {
	return m.values.admissionWebhook
}

func (m MonitorRuntime) Ingress() IngressMonitor { return m.values.ingress }

func (m MonitorRuntime) NetworkPolicy() NetworkPolicyMonitor {
	return m.values.networkPolicy
}

func (m MonitorRuntime) ControlPlane() ControlPlaneMonitor {
	return m.values.controlPlane
}

func (m MonitorRuntime) ClusterAutoscaler() ClusterAutoscalerMonitor {
	return m.values.clusterAutoscaler
}

func (m MonitorRuntime) Job() JobMonitor { return m.values.job }

func (m MonitorRuntime) PVC() PvcMonitor { return m.values.pvc }

func (m MonitorRuntime) Heartbeat() HeartbeatMonitor {
	return m.values.heartbeat
}

func (m MonitorRuntime) Metrics() RuntimeMetricsMonitor {
	return m.values.runtimeMetrics
}

func (m MonitorRuntime) ActiveProbe() ActiveProbeMonitor {
	return cloneActiveProbeMonitor(m.values.activeProbe)
}

func (m MonitorRuntime) KubeletTelemetry() KubeletTelemetryMonitor {
	return m.values.kubeletTelemetry
}

func (m MonitorRuntime) CRD() CrdConfig {
	return CrdConfig{
		Enabled:           m.values.crd.Enabled,
		FailureConditions: cloneStrings(m.values.crd.FailureConditions),
		GraphReferences:   cloneStrings(m.values.crd.GraphReferences),
	}
}

func (m MonitorRuntime) ClusterResource() ClusterResourceMonitor {
	return m.values.clusterResource
}

func (m MonitorRuntime) TLS() TlsMonitor { return m.values.tls }

func (m MonitorRuntime) Rollout() RolloutMonitor { return m.values.rollout }

func (m MonitorRuntime) DaemonSet() DaemonSetMonitor {
	return m.values.daemonSet
}

func (m MonitorRuntime) StatefulSet() StatefulSetMonitor {
	return m.values.statefulSet
}

func (m MonitorRuntime) CronJob() CronJobMonitor { return m.values.cronJob }

func (m MonitorRuntime) HPA() HpaMonitor { return m.values.hpa }

func (m MonitorRuntime) PDB() PdbMonitor { return m.values.pdb }

func (m MonitorRuntime) AdaptiveThresholds() bool {
	return m.values.adaptiveThresholds
}

func (m MonitorRuntime) ContainerRestartThreshold() int {
	return m.values.containerRestart
}

func (m MonitorRuntime) Schedule() ScheduleMonitor {
	return m.values.schedule
}

func (m MonitorRuntime) OOM() OomMonitor { return m.values.oom }

func (m MonitorRuntime) IncludeEvents() bool { return m.values.includeEvents }

func (m MonitorRuntime) IncludeLogs() bool { return m.values.includeLogs }

func (m MonitorRuntime) Maintenance() MaintenanceConfig {
	return m.values.maintenance
}

func (m MonitorRuntime) PendingPod() PendingPodMonitor {
	return m.values.pendingPod
}

func (m MonitorRuntime) NotReady() NotReadyMonitor {
	return m.values.notReady
}

func (m MonitorRuntime) IgnoreDisruptions() bool {
	return m.values.ignoreDisruptions
}

func (m MonitorRuntime) MaxRecentLogLines() int64 {
	return m.values.maxRecentLogLines
}

func (m MonitorRuntime) IgnoreGracefulKill() bool {
	return m.values.ignoreGracefulKill
}

func (m MonitorRuntime) ReportStartup() bool { return m.values.reportStartup }

// DeliveryRuntime groups normalized provider and routing policy.
type DeliveryRuntime struct {
	values runtimeDelivery
}

func (d DeliveryRuntime) ProviderNames() []string {
	return cloneStrings(d.values.providerNames)
}

func (d DeliveryRuntime) Providers() []ProviderRuntime {
	return cloneProviderRuntimes(d.values.providers)
}

func (d DeliveryRuntime) Silences() []SilenceRule {
	return cloneSilenceRules(d.values.silences)
}

func (d DeliveryRuntime) Templates() map[string]string {
	return cloneStringMap(d.values.templates)
}

func (d DeliveryRuntime) Runbooks() map[string]string {
	return cloneStringMap(d.values.runbooks)
}

func (d DeliveryRuntime) IncludePrivateLogAddresses() bool {
	return d.values.includePrivateLogAddresses
}

// LifecycleRuntime groups lifecycle, health, telemetry, and worker settings.
type LifecycleRuntime struct {
	values runtimeOperations
}

func (o LifecycleRuntime) Telemetry() Telemetry {
	return o.values.telemetry
}

func (o LifecycleRuntime) Upgrader() Upgrader { return o.values.upgrader }

func (o LifecycleRuntime) HealthCheck() HealthCheck {
	return o.values.healthCheck
}

func (o LifecycleRuntime) AuditLog() AuditLogConfig {
	return o.values.auditLog
}

func (o LifecycleRuntime) ResyncInterval() time.Duration {
	return o.values.resync
}

func (o LifecycleRuntime) Workers() int { return o.values.workers }

func (o LifecycleRuntime) WatchStartTime() time.Time {
	return o.values.watchStart
}

// PersistenceRuntime groups persistence limits and policies.
type PersistenceRuntime struct {
	values runtimePersistence
}

func (p PersistenceRuntime) MaxBaseline() int {
	return p.values.maxBaseline
}

func (i IncidentRuntime) SeverityByOwnerKind() map[string]string {
	return cloneStringMap(i.severityByOwnerKind)
}

func (i IncidentRuntime) SeverityByReason() map[string]string {
	return cloneStringMap(i.severityByReason)
}

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

func (r RuntimeConfig) Scope() ScopeRuntime {
	return ScopeRuntime{values: cloneRuntimeScope(r.scope)}
}

func (r RuntimeConfig) Monitors() MonitorRuntime {
	return MonitorRuntime{values: cloneRuntimeMonitors(r.monitors)}
}

func (r RuntimeConfig) Delivery() DeliveryRuntime {
	return DeliveryRuntime{values: cloneRuntimeDelivery(r.delivery)}
}

// Lifecycle returns the grouped application lifecycle settings.
func (r RuntimeConfig) Lifecycle() LifecycleRuntime {
	return LifecycleRuntime{values: r.operations}
}

func (r RuntimeConfig) Persistence() PersistenceRuntime {
	return PersistenceRuntime{values: r.persistence}
}

func cloneRuntimeScope(value runtimeScope) runtimeScope {
	value.allowedNamespaces = cloneStrings(value.allowedNamespaces)
	value.forbiddenNamespaces = cloneStrings(value.forbiddenNamespaces)
	value.allowedReasons = cloneStrings(value.allowedReasons)
	value.forbiddenReasons = cloneStrings(value.forbiddenReasons)
	value.suppression = cloneSuppressionIndex(value.suppression)
	return value
}

func cloneRuntimeMonitors(value runtimeMonitors) runtimeMonitors {
	value.activeProbe = cloneActiveProbeMonitor(value.activeProbe)
	value.crd.FailureConditions = cloneStrings(value.crd.FailureConditions)
	value.crd.GraphReferences = cloneStrings(value.crd.GraphReferences)
	return value
}

func cloneRuntimeDelivery(value runtimeDelivery) runtimeDelivery {
	value.providerNames = cloneStrings(value.providerNames)
	value.providers = cloneProviderRuntimes(value.providers)
	value.silences = cloneSilenceRules(value.silences)
	value.templates = cloneStringMap(value.templates)
	value.runbooks = cloneStringMap(value.runbooks)
	return value
}
