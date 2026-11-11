package config

import (
	"sort"
	"time"
)

// RuntimeConfig is the immutable, derived configuration used at runtime.
// The YAML-facing Config is confined to loading and overlay boundaries; this
// value keeps normalization out of component construction.
type RuntimeConfig struct {
	compiled    bool
	application ApplicationRuntime
	lifecycle   runtimeLifecycle
	scope       runtimeScope
	delivery    runtimeDelivery
	policy      runtimePolicy
	activeProbe ActiveProbeMonitor
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
	// HideProHints removes the Pro note from Community messages
	// (app.hideProHints).
	HideProHints bool
	// NonProduction is set when app.environment marks the cluster as
	// not production: nothing in it pages.
	NonProduction bool
	// KubeletInsecureSkipVerify skips kubelet serving certificate
	// verification (kubelet.insecureSkipVerify).
	KubeletInsecureSkipVerify bool
	// WatchSecrets is false when Secrets must not be watched
	// (watch.secrets).
	WatchSecrets bool
	// InvestigationLogs is false when evidence must not read logs
	// (investigation.logs).
	InvestigationLogs bool
	// InvestigationSampleLogs is false when running pods' logs must not
	// be sampled for error bursts (investigation.sampleLogs).
	InvestigationSampleLogs bool
}

// These private views keep the snapshot navigable without exposing mutable
// maps and slices as public fields. Accessors return defensive copies.
type runtimeLifecycle struct {
	telemetry   Telemetry
	license     License
	upgrader    Upgrader
	healthCheck HealthCheck
	dashboard   Dashboard
	auditLog    AuditLogConfig
	heartbeat   HeartbeatMonitor
	crdEnabled  bool
	resync      time.Duration
	oldProblem  time.Duration
}

type runtimeScope struct {
	allowedNamespaces   []string
	forbiddenNamespaces []string
	namespaceSelector   string
	allowedReasons      []string
	forbiddenReasons    []string
	silences            []SilenceRule
}

type runtimeDelivery struct {
	providerNames []string
	providers     []ProviderRuntime
	templates     map[string]string
}

type runtimePolicy struct {
	severityByReason    map[string]string
	severityByOwnerKind map[string]string
	runbooks            map[string]string
	maintenance         MaintenanceConfig
}

// activeProviderNames lists the configured providers in a stable order,
// leaving out removed ones whose section is only kept so the file loads.
func activeProviderNames(alert map[string]map[string]interface{}) []string {
	names := make([]string, 0, len(alert))
	for name := range alert {
		if !isRemovedProvider(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// CompileRuntimeConfig creates a defensive snapshot of derived settings.
func CompileRuntimeConfig(c *Config) RuntimeConfig {
	if c == nil {
		return RuntimeConfig{}
	}
	providers := activeProviderNames(c.Alert)
	return RuntimeConfig{
		compiled:    true,
		application: applicationRuntime(c),
		lifecycle: runtimeLifecycle{
			telemetry:   c.Telemetry,
			license:     c.License,
			upgrader:    c.Upgrader,
			healthCheck: c.HealthCheck,
			dashboard:   c.Dashboard,
			auditLog:    c.AuditLog,
			heartbeat:   c.HeartbeatMonitor,
			crdEnabled:  c.CrdConfig.Enabled,
			resync:      time.Duration(c.ResyncSeconds) * time.Second,
			oldProblem:  oldProblemHorizon(c.OldProblemDays),
		},
		scope: runtimeScope{
			allowedNamespaces:   cloneStrings(c.AllowedNamespaces),
			forbiddenNamespaces: cloneStrings(c.ForbiddenNamespaces),
			namespaceSelector:   c.NamespaceSelector,
			allowedReasons:      cloneStrings(c.AllowedReasons),
			forbiddenReasons:    cloneStrings(c.ForbiddenReasons),
			silences:            cloneSilenceRules(c.Silences),
		},
		delivery: runtimeDelivery{
			providerNames: providers,
			providers:     compileProviderRuntimes(c.Alert, providers),
			templates:     cloneStringMap(c.Templates),
		},
		policy: runtimePolicy{
			severityByReason:    cloneStringMap(c.SeverityByReason),
			severityByOwnerKind: cloneStringMap(c.SeverityByOwnerKind),
			runbooks:            cloneStringMap(c.Runbooks),
			maintenance:         c.Maintenance,
		},
		activeProbe: cloneActiveProbeMonitor(c.ActiveProbeMonitor),
	}
}

// applicationRuntime is the application part of the snapshot.
func applicationRuntime(c *Config) ApplicationRuntime {
	return ApplicationRuntime{
		ProxyURL:                  c.App.ProxyURL,
		ClusterName:               c.App.ClusterName,
		DisableStartupMessage:     c.App.DisableStartupMessage,
		LogFormatter:              c.App.LogFormatter,
		InsecureSkipTLSVerify:     c.App.InsecureSkipTLSVerify,
		CABundlePath:              c.App.CABundlePath,
		HideProHints:              c.App.HideProHints,
		NonProduction:             c.App.NonProduction(),
		KubeletInsecureSkipVerify: c.Kubelet.InsecureSkipVerify,
		WatchSecrets:              c.Watch.Secrets,
		InvestigationLogs:         c.Investigation.Logs,
		InvestigationSampleLogs:   c.Investigation.SampleLogs,
	}
}

// maxOldProblemDays bounds oldProblemDays at a century. A larger value
// means "never" just as well, and past 106,751 days the conversion to
// a duration overflows: it would turn the setting off, or into minutes.
const maxOldProblemDays = 36500

// oldProblemHorizon is oldProblemDays as a duration, clamped instead
// of rejected so a very large value that worked keeps kwatch starting.
func oldProblemHorizon(days int) time.Duration {
	return time.Duration(min(days, maxOldProblemDays)) * 24 * time.Hour
}
