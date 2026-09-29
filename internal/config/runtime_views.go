package config

import "time"

// ScopeRuntime is the namespace, reason, selector and silence policy.
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

// Silences includes the rules converted from deprecated ignore* fields.
func (s ScopeRuntime) Silences() []SilenceRule {
	return cloneSilenceRules(s.values.silences)
}

// DeliveryRuntime groups normalized providers and message templates.
type DeliveryRuntime struct {
	values runtimeDelivery
}

func (d DeliveryRuntime) ProviderNames() []string {
	return cloneStrings(d.values.providerNames)
}

func (d DeliveryRuntime) Providers() []ProviderRuntime {
	return cloneProviderRuntimes(d.values.providers)
}

func (d DeliveryRuntime) Templates() map[string]string {
	return cloneStringMap(d.values.templates)
}

// PolicyRuntime is how the operator adjusts problems: severity overrides,
// runbook links and maintenance annotations.
type PolicyRuntime struct {
	values runtimePolicy
}

func (p PolicyRuntime) SeverityByReason() map[string]string {
	return cloneStringMap(p.values.severityByReason)
}

func (p PolicyRuntime) SeverityByOwnerKind() map[string]string {
	return cloneStringMap(p.values.severityByOwnerKind)
}

func (p PolicyRuntime) Runbooks() map[string]string {
	return cloneStringMap(p.values.runbooks)
}

func (p PolicyRuntime) Maintenance() MaintenanceConfig {
	return p.values.maintenance
}

// LifecycleRuntime groups health, telemetry, upgrade, audit, heartbeat,
// CRD and resync settings.
type LifecycleRuntime struct {
	values runtimeLifecycle
}

func (o LifecycleRuntime) Telemetry() Telemetry { return o.values.telemetry }

func (o LifecycleRuntime) Upgrader() Upgrader { return o.values.upgrader }

func (o LifecycleRuntime) HealthCheck() HealthCheck {
	return o.values.healthCheck
}

func (o LifecycleRuntime) AuditLog() AuditLogConfig {
	return o.values.auditLog
}

func (o LifecycleRuntime) Heartbeat() HeartbeatMonitor {
	return o.values.heartbeat
}

// CRDEnabled reports whether KwatchConfig resources overlay the config.
func (o LifecycleRuntime) CRDEnabled() bool { return o.values.crdEnabled }

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

// Policy returns the operator's problem adjustments.
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
