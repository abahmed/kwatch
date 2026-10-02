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
	// KubeletInsecureSkipVerify skips kubelet serving certificate
	// verification (kubelet.insecureSkipVerify).
	KubeletInsecureSkipVerify bool
	// WatchSecrets is false when Secrets must not be watched
	// (watch.secrets).
	WatchSecrets bool
}

// These private views keep the snapshot navigable without exposing mutable
// maps and slices as public fields. Accessors return defensive copies.
type runtimeLifecycle struct {
	telemetry   Telemetry
	upgrader    Upgrader
	healthCheck HealthCheck
	auditLog    AuditLogConfig
	heartbeat   HeartbeatMonitor
	crdEnabled  bool
	resync      time.Duration
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

// CompileRuntimeConfig creates a defensive snapshot of derived settings.
func CompileRuntimeConfig(c *Config) RuntimeConfig {
	if c == nil {
		return RuntimeConfig{}
	}
	providers := make([]string, 0, len(c.Alert))
	for name := range c.Alert {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return RuntimeConfig{
		compiled: true,
		application: ApplicationRuntime{
			ProxyURL:                  c.App.ProxyURL,
			ClusterName:               c.App.ClusterName,
			DisableStartupMessage:     c.App.DisableStartupMessage,
			LogFormatter:              c.App.LogFormatter,
			InsecureSkipTLSVerify:     c.App.InsecureSkipTLSVerify,
			CABundlePath:              c.App.CABundlePath,
			KubeletInsecureSkipVerify: c.Kubelet.InsecureSkipVerify,
			WatchSecrets:              c.Watch.Secrets,
		},
		lifecycle: runtimeLifecycle{
			telemetry:   c.Telemetry,
			upgrader:    c.Upgrader,
			healthCheck: c.HealthCheck,
			auditLog:    c.AuditLog,
			heartbeat:   c.HeartbeatMonitor,
			crdEnabled:  c.CrdConfig.Enabled,
			resync:      time.Duration(c.ResyncSeconds) * time.Second,
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
