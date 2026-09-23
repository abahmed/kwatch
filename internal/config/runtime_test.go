package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCompileRuntimeConfigCopiesDerivedValues(t *testing.T) {
	cfg := &Config{
		Alert:             map[string]map[string]interface{}{"Slack": {}},
		AllowedNamespaces: []string{"team-a"},
		ForbiddenReasons:  []string{"Evicted"},
		ResyncSeconds:     17,
		Workers:           3,
		NamespaceSelector: "team=platform",
		Correlation:       Correlation{MaxBaseline: 42},
		NodeMonitor:       NodeMonitor{Enabled: true},
		Templates:         map[string]string{"warning": "{{.Message}}"},
		Runbooks:          map[string]string{"CrashLoopBackOff": "https://runbook"},
		Silences: []SilenceRule{{
			Namespaces: []string{"team-a"},
			Reasons:    []string{"Evicted"},
		}},
		Suppression: SuppressionIndex{
			ContainerNames: []string{"sidecar"},
		},
	}

	runtime := CompileRuntimeConfig(cfg)
	allowed := runtime.Scope().AllowedNamespaces()
	providers := runtime.Delivery().ProviderNames()
	allowed[0] = "changed"
	providers[0] = "changed"

	require.Equal(t, []string{"team-a"}, runtime.Scope().AllowedNamespaces())
	require.Equal(t, []string{"Slack"}, runtime.Delivery().ProviderNames())
	require.Equal(t, []string{"Evicted"}, runtime.Scope().ForbiddenReasons())
	require.Equal(t, 17*time.Second, runtime.Lifecycle().ResyncInterval())
	require.Equal(t, 3, runtime.Lifecycle().Workers())
	require.Equal(t, "team=platform", runtime.Scope().NamespaceSelector())
	require.Equal(t, 42, runtime.Persistence().MaxBaseline())
	require.True(t, runtime.Monitors().Node().Enabled)
	templates := runtime.Delivery().Templates()
	runbooks := runtime.Delivery().Runbooks()
	silences := runtime.Delivery().Silences()
	templates["warning"] = "changed"
	runbooks["CrashLoopBackOff"] = "changed"
	silences[0].Namespaces[0] = "changed"
	require.Equal(t, "{{.Message}}", runtime.Delivery().Templates()["warning"])
	require.Equal(
		t, "https://runbook", runtime.Delivery().Runbooks()["CrashLoopBackOff"],
	)
	require.Equal(t, "team-a", runtime.Delivery().Silences()[0].Namespaces[0])
}

func TestCompileRuntimeConfigCopiesProviderTemplates(t *testing.T) {
	cfg := &Config{Alert: map[string]map[string]interface{}{
		"webhook": {
			"templates": map[string]interface{}{
				"warning": "{{.Message}}",
			},
		},
	}}

	runtime := CompileRuntimeConfig(cfg)
	providers := runtime.Delivery().Providers()
	require.Len(t, providers, 1)
	providers[0].Templates["warning"] = "changed"

	actual := runtime.Delivery().Providers()
	require.Equal(t, "{{.Message}}", actual[0].Templates["warning"])
}

func TestCompileRuntimeConfigCapturesSharedOutputPolicy(t *testing.T) {
	includeEvents := false
	includeLogs := true
	cfg := &Config{
		IncludeEvents:         &includeEvents,
		IncludeLogs:           &includeLogs,
		Maintenance:           MaintenanceConfig{Enabled: true, Annotation: "ops"},
		MaxRecentLogLines:     25,
		ReportStartupBaseline: true,
	}

	runtime := CompileRuntimeConfig(cfg)

	require.False(t, runtime.Monitors().IncludeEvents())
	require.True(t, runtime.Monitors().IncludeLogs())
	require.Equal(t, "ops", runtime.Monitors().Maintenance().Annotation)
	require.Equal(t, int64(25), runtime.Monitors().MaxRecentLogLines())
	require.True(t, runtime.Monitors().ReportStartup())
}

func TestCompileRuntimeConfigCapturesMessagePolicy(t *testing.T) {
	cfg := &Config{Message: MessageConfig{
		IncludePrivateLogAddresses: true,
	}}

	runtime := CompileRuntimeConfig(cfg)

	require.True(t, runtime.Delivery().IncludePrivateLogAddresses())
}

func TestCompileRuntimeConfigCopiesIntegrationPolicies(t *testing.T) {
	cfg := &Config{
		PvcMonitor:       PvcMonitor{Enabled: true, Threshold: 80},
		HeartbeatMonitor: HeartbeatMonitor{Enabled: true, URL: "https://hb"},
		RuntimeMetricsMonitor: RuntimeMetricsMonitor{
			Enabled: true, MemoryWarningPercent: 80,
		},
		ActiveProbeMonitor: ActiveProbeMonitor{
			Enabled:           true,
			HTTP:              []HTTPProbeTarget{{Name: "api", URL: "https://api"}},
			ExcludeNamespaces: []string{"noisy"},
		},
		KubeletTelemetryMonitor: KubeletTelemetryMonitor{
			Enabled: true, PersistState: true,
		},
		CrdConfig: CrdConfig{
			Enabled: true, FailureConditions: []string{"Ready=False"},
		},
	}

	runtime := CompileRuntimeConfig(cfg)
	probe := runtime.Monitors().ActiveProbe()
	conditions := runtime.Monitors().CRD()
	probe.HTTP[0].URL = "changed"
	probe.ExcludeNamespaces[0] = "changed"
	conditions.FailureConditions[0] = "changed"

	require.True(t, runtime.Monitors().PVC().Enabled)
	require.Equal(t, "https://hb", runtime.Monitors().Heartbeat().URL)
	require.True(t, runtime.Monitors().Metrics().Enabled)
	require.True(t, runtime.Monitors().KubeletTelemetry().PersistState)
	require.Equal(t, "https://api", runtime.Monitors().ActiveProbe().HTTP[0].URL)
	require.Equal(t, []string{"noisy"},
		runtime.Monitors().ActiveProbe().ExcludeNamespaces)
	require.Equal(t, []string{"Ready=False"},
		runtime.Monitors().CRD().FailureConditions)
}

func TestRuntimeConfigForCompilesDirectConfigurationWithoutMutation(
	t *testing.T,
) {
	includeEvents := false
	cfg := &Config{
		IncludeEvents:     &includeEvents,
		AllowedNamespaces: []string{"team-a"},
		ResyncSeconds:     9,
		Alert:             map[string]map[string]interface{}{"Webhook": {}},
	}

	runtime := RuntimeConfigFor(cfg)

	require.False(t, runtime.Monitors().IncludeEvents())
	require.Equal(t, []string{"team-a"}, runtime.Scope().AllowedNamespaces())
	require.Equal(t, []string{"Webhook"}, runtime.Delivery().ProviderNames())
	require.Equal(t, 9*time.Second, runtime.Lifecycle().ResyncInterval())
	require.False(t, cfg.Runtime.Compiled())
}

func TestRuntimeConfigCompilesProviderDeliveryPolicy(t *testing.T) {
	cfg := &Config{Alert: map[string]map[string]interface{}{
		"Webhook": {
			"url":      "https://example.test/hook",
			"fallback": "Slack",
			"retry": map[string]interface{}{
				"maxAttempts": 5,
				"delay":       "2s",
				"maxBackoff":  "10s",
			},
			"routes": []interface{}{map[string]interface{}{
				"namespaces": []interface{}{"ops"},
				"severities": []interface{}{"high"},
			}},
		},
	}}

	providers := CompileRuntimeConfig(cfg).Delivery().Providers()
	require.Len(t, providers, 1)
	require.Equal(t, "Webhook", providers[0].Name)
	require.Equal(t, "Slack", providers[0].FallbackName)
	require.Equal(t, 5, providers[0].Retry.MaxAttempts)
	require.Equal(t, 2*time.Second, providers[0].Retry.Delay)
	require.Equal(t, 10*time.Second, providers[0].Retry.MaxBackoff)
	require.Equal(t, []string{"ops"}, providers[0].Routes[0].Namespaces)

	providers[0].Settings["url"] = "changed"
	providers[0].Routes[0].Namespaces[0] = "changed"
	fresh := CompileRuntimeConfig(cfg).Delivery().Providers()
	require.Equal(t, "https://example.test/hook", fresh[0].Settings["url"])
	require.Equal(t, []string{"ops"}, fresh[0].Routes[0].Namespaces)
}

func TestRuntimeConfigCopiesIncidentPolicies(t *testing.T) {
	start := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	cfg := &Config{
		WatchStartTime: start,
		SeverityByOwnerKind: map[string]string{
			"Deployment": "high",
		},
		SeverityByReason: map[string]string{
			"Evicted": "normal",
		},
		Correlation: Correlation{
			Escalation: EscalationConfig{Tiers: []int{2, 4}},
			Renotify: RenotifyConfig{
				IntervalBySeverity: map[string]int{"high": 5},
			},
		},
	}

	runtime := CompileRuntimeConfig(cfg)
	incident := runtime.Incident()
	incident.EscalationTiers[0] = 99
	incident.RenotifyIntervalBySeverity["high"] = time.Hour
	owners := runtime.Incident().SeverityByOwnerKind()
	reasons := runtime.Incident().SeverityByReason()
	owners["Deployment"] = "low"
	reasons["Evicted"] = "high"

	require.Equal(t, []int{2, 4}, runtime.Incident().EscalationTiers)
	require.Equal(
		t, 5*time.Minute,
		runtime.Incident().RenotifyIntervalBySeverity["high"],
	)
	require.Equal(
		t, "high", runtime.Incident().SeverityByOwnerKind()["Deployment"],
	)
	require.Equal(t, "normal", runtime.Incident().SeverityByReason()["Evicted"])
	require.Equal(t, start, runtime.Lifecycle().WatchStartTime())
}
