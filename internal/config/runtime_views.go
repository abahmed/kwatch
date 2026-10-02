package config

import "time"

// ScopeRuntime is the namespace, reason, selector and silence policy.
type ScopeRuntime struct {
	values runtimeScope
}

// AllowedNamespaces returns the namespaces to watch.
func (s ScopeRuntime) AllowedNamespaces() []string {
	return cloneStrings(s.values.allowedNamespaces)
}

// ForbiddenNamespaces returns the namespaces never watched.
func (s ScopeRuntime) ForbiddenNamespaces() []string {
	return cloneStrings(s.values.forbiddenNamespaces)
}

// NamespaceSelector returns the namespace label selector.
func (s ScopeRuntime) NamespaceSelector() string {
	return s.values.namespaceSelector
}

// AllowedReasons returns the reasons that may alert.
func (s ScopeRuntime) AllowedReasons() []string {
	return cloneStrings(s.values.allowedReasons)
}

// ForbiddenReasons returns the reasons that never alert.
func (s ScopeRuntime) ForbiddenReasons() []string {
	return cloneStrings(s.values.forbiddenReasons)
}

// Silences includes the rules converted from deprecated ignore* fields.
func (s ScopeRuntime) Silences() []SilenceRule {
	return cloneSilenceRules(s.values.silences)
}

// DeliveryRuntime groups normalized providers and message templates.
type DeliveryRuntime struct {
	values runtimeDelivery
}

// ProviderNames returns the configured provider names.
func (d DeliveryRuntime) ProviderNames() []string {
	return cloneStrings(d.values.providerNames)
}

// Providers returns the normalized provider settings.
func (d DeliveryRuntime) Providers() []ProviderRuntime {
	return cloneProviderRuntimes(d.values.providers)
}

// Templates returns the message templates by name.
func (d DeliveryRuntime) Templates() map[string]string {
	return cloneStringMap(d.values.templates)
}

// PolicyRuntime is how the operator adjusts incidents: severity overrides,
// runbook links and maintenance annotations.
type PolicyRuntime struct {
	values runtimePolicy
}

// SeverityByReason returns severity overrides by reason.
func (p PolicyRuntime) SeverityByReason() map[string]string {
	return cloneStringMap(p.values.severityByReason)
}

// SeverityByOwnerKind returns severity overrides by owner kind.
func (p PolicyRuntime) SeverityByOwnerKind() map[string]string {
	return cloneStringMap(p.values.severityByOwnerKind)
}

// Runbooks returns runbook links by reason.
func (p PolicyRuntime) Runbooks() map[string]string {
	return cloneStringMap(p.values.runbooks)
}

// Maintenance returns the maintenance annotation settings.
func (p PolicyRuntime) Maintenance() MaintenanceConfig {
	return p.values.maintenance
}

// LifecycleRuntime groups health, telemetry, upgrade, audit, heartbeat,
// CRD and resync settings.
type LifecycleRuntime struct {
	values runtimeLifecycle
}

// Telemetry returns the telemetry settings.
func (o LifecycleRuntime) Telemetry() Telemetry { return o.values.telemetry }

// Upgrader returns the upgrade check settings.
func (o LifecycleRuntime) Upgrader() Upgrader { return o.values.upgrader }

// HealthCheck returns the health check settings.
func (o LifecycleRuntime) HealthCheck() HealthCheck {
	return o.values.healthCheck
}

// AuditLog returns the audit log settings.
func (o LifecycleRuntime) AuditLog() AuditLogConfig {
	return o.values.auditLog
}

// Heartbeat returns the heartbeat settings.
func (o LifecycleRuntime) Heartbeat() HeartbeatMonitor {
	return o.values.heartbeat
}

// CRDEnabled reports whether KwatchConfig resources overlay the config.
func (o LifecycleRuntime) CRDEnabled() bool { return o.values.crdEnabled }

// ResyncInterval returns the informer resync interval.
func (o LifecycleRuntime) ResyncInterval() time.Duration {
	return o.values.resync
}

// Scope returns the scope policy. Accessors copy on read.
func (r RuntimeConfig) Scope() ScopeRuntime {
	return ScopeRuntime{values: r.scope}
}

// Delivery returns providers and templates. Accessors copy on read.
func (r RuntimeConfig) Delivery() DeliveryRuntime {
	return DeliveryRuntime{values: r.delivery}
}

// Policy returns the operator's incident adjustments.
func (r RuntimeConfig) Policy() PolicyRuntime {
	return PolicyRuntime{values: r.policy}
}

// Lifecycle returns the grouped application lifecycle settings.
func (r RuntimeConfig) Lifecycle() LifecycleRuntime {
	return LifecycleRuntime{values: r.lifecycle}
}

// ActiveProbe returns the configured probe targets.
func (r RuntimeConfig) ActiveProbe() ActiveProbeMonitor {
	return cloneActiveProbeMonitor(r.activeProbe)
}
