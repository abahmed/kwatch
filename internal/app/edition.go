package app

import (
	"sort"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// Pro configuration that no catalog capability names. They use IDs of
// their own; an ID outside the catalog is Pro, so Community never has
// them.
const (
	reasonFilters     feature.ID = "policy.reason-filter"
	namespaceSelector feature.ID = "policy.namespace-selector"
	heartbeatMonitor  feature.ID = "delivery.heartbeat"
	// extraProviders stands for alert providers beyond the edition's
	// cap; inactivePro reports it by name, not by this ID.
	extraProviders feature.ID = "delivery.providers"
	// FlightRecorder records observations for replay-export.
	FlightRecorder feature.ID = "diagnostics.flight-recorder"
)

// defaultMaintenanceAnnotation and defaultMaintenanceUntil are the
// annotations the built-in configuration uses. Maintenance is on by
// default, so only different annotations show that someone set it up.
const (
	defaultMaintenanceAnnotation = "kwatch.io/maintenance"
	defaultMaintenanceUntil      = "kwatch.io/maintenance-until"
)

// ResolveEntitlements is what this installation may use. Until licenses
// exist it is always the Community edition; a test build may override it
// (see edition_override.go).
func ResolveEntitlements() feature.Entitlements {
	return editionOverride(feature.CommunityEntitlements())
}

// entitled reports whether the installation may use id. A nil value is
// the Community edition: a missed wiring never grants Pro.
func entitled(e feature.Entitlements, id feature.ID) bool {
	return feature.OrCommunity(e).Has(id)
}

// configuredPro lists the Pro features cfg sets, sorted. It reads only
// what the operator chose: a built-in default is not a choice.
func configuredPro(cfg *config.Config) []feature.ID {
	if cfg == nil {
		return nil
	}
	var ids []feature.ID
	add := func(on bool, id feature.ID) {
		if on {
			ids = append(ids, id)
		}
	}
	probe := cfg.ActiveProbeMonitor
	add(probe.Enabled || len(probe.HTTP)+len(probe.TCP)+len(probe.DNS) > 0,
		feature.ActiveProbes)
	// The audit log is on by default; a file output is a choice.
	add(cfg.AuditLog.Output != "" && cfg.AuditLog.Output != "stdout",
		feature.AuditLog)
	add(cfg.HeartbeatMonitor.Enabled || cfg.HeartbeatMonitor.URL != "",
		heartbeatMonitor)
	add(len(cfg.SeverityByReason)+len(cfg.SeverityByOwnerKind) > 0,
		feature.SeverityOverrides)
	add(customMaintenance(cfg.Maintenance), feature.Maintenance)
	add(len(cfg.Runbooks) > 0, feature.Runbooks)
	add(len(cfg.Reasons) > 0, reasonFilters)
	add(cfg.NamespaceSelector != "", namespaceSelector)
	for _, id := range deliveryPro(cfg) {
		add(true, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func customMaintenance(m config.MaintenanceConfig) bool {
	return m.Annotation != defaultMaintenanceAnnotation ||
		m.UntilAnnotation != defaultMaintenanceUntil
}

// inactivePro names the Pro features cfg sets that e does not run, the
// list /status and the startup warning show.
func inactivePro(e feature.Entitlements, cfg *config.Config) []string {
	e = feature.OrCommunity(e)
	var names []string
	for _, id := range feature.Inactive(e, configuredPro(cfg)) {
		if id == extraProviders {
			continue
		}
		names = append(names, string(id))
	}
	if _, dropped := capProviders(e, cfg); len(dropped) > 0 {
		names = append(names, providerCapNotice(e, dropped))
	}
	sort.Strings(names)
	return names
}

// logInactivePro warns once, in one line, about Pro configuration that
// is set and not run. Community never fails because of it.
func logInactivePro(e feature.Entitlements, names []string) {
	if len(names) == 0 {
		return
	}
	klog.Warningf("configuration sets kwatch Pro features that this "+
		"edition does not run; they are ignored: component=edition "+
		"edition=%s features=%q", feature.OrCommunity(e).Edition(), names)
}

// restrictConfig returns cfg without the Pro policy e does not run. The
// result is a copy; cfg is untouched. Namespace include and exclude and
// silences stay: they are Community scope.
func restrictConfig(cfg *config.Config, e feature.Entitlements) *config.Config {
	if cfg == nil {
		return cfg
	}
	e = feature.OrCommunity(e)
	out := *cfg
	if !e.Has(feature.SeverityOverrides) {
		out.SeverityByReason, out.SeverityByOwnerKind = nil, nil
	}
	if !e.Has(feature.Maintenance) {
		out.Maintenance.Enabled = false
	}
	if !e.Has(feature.Runbooks) {
		out.Runbooks = nil
	}
	if !e.Has(reasonFilters) {
		out.Reasons = nil
		out.AllowedReasons, out.ForbiddenReasons = nil, nil
	}
	if !e.Has(namespaceSelector) {
		out.NamespaceSelector = ""
	}
	out.Alert, out.Templates = restrictDelivery(cfg, e)
	if !e.Has(feature.AuditLog) {
		out.AuditLog.Enabled = false
	}
	if !e.Has(heartbeatMonitor) {
		out.HeartbeatMonitor.Enabled = false
	}
	if !e.Has(feature.ActiveProbes) {
		out.ActiveProbeMonitor.Enabled = false
	}
	out.Runtime = config.CompileRuntimeConfig(&out)
	return &out
}

// applyEdition returns cfg restricted to the edition, and the names of
// the Pro features cfg sets that the edition does not run.
func applyEdition(
	cfg *config.Config, e feature.Entitlements,
) (*config.Config, []string) {
	return restrictConfig(cfg, e), inactivePro(e, cfg)
}
