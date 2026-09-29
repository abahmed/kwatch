package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCompileRuntimeConfigCopiesDerivedValues(t *testing.T) {
	cfg := &Config{
		App:                 App{ClusterName: "prod"},
		Alert:               map[string]map[string]interface{}{"Slack": {}},
		AllowedNamespaces:   []string{"team-a"},
		ForbiddenReasons:    []string{"Evicted"},
		NamespaceSelector:   "team=platform",
		ResyncSeconds:       17,
		CrdConfig:           CrdConfig{Enabled: true},
		HeartbeatMonitor:    HeartbeatMonitor{Enabled: true, Interval: 60},
		Templates:           map[string]string{"error": "{{.Text}}"},
		Runbooks:            map[string]string{"OOMKilled": "https://runbook"},
		SeverityByReason:    map[string]string{"OOMKilled": "critical"},
		SeverityByOwnerKind: map[string]string{"StatefulSet": "critical"},
		Maintenance:         MaintenanceConfig{Enabled: true, Annotation: "m"},
		Silences:            []SilenceRule{{Namespaces: []string{"batch"}}},
		ActiveProbeMonitor: ActiveProbeMonitor{
			Enabled: true, HTTP: []HTTPProbeTarget{{Name: "api"}},
		},
	}

	runtime := CompileRuntimeConfig(cfg)

	require.Equal(t, "prod", runtime.Application().ClusterName)
	require.Equal(t, []string{"team-a"}, runtime.Scope().AllowedNamespaces())
	require.Equal(t, []string{"Evicted"}, runtime.Scope().ForbiddenReasons())
	require.Equal(t, "team=platform", runtime.Scope().NamespaceSelector())
	require.Equal(t, 17*time.Second, runtime.Lifecycle().ResyncInterval())
	require.True(t, runtime.Lifecycle().CRDEnabled())
	require.Equal(t, 60, runtime.Lifecycle().Heartbeat().Interval)
	require.Equal(t, []string{"Slack"}, runtime.Delivery().ProviderNames())
	require.Equal(t, "{{.Text}}", runtime.Delivery().Templates()["error"])
	require.Equal(t, "critical",
		runtime.Policy().SeverityByReason()["OOMKilled"])
	require.Equal(t, "critical",
		runtime.Policy().SeverityByOwnerKind()["StatefulSet"])
	require.Equal(t, "https://runbook",
		runtime.Policy().Runbooks()["OOMKilled"])
	require.Equal(t, "m", runtime.Policy().Maintenance().Annotation)
	require.Equal(t, "api", runtime.ActiveProbe().HTTP[0].Name)
}

func TestRuntimeConfigViewsAreDefensive(t *testing.T) {
	cfg := &Config{
		AllowedNamespaces: []string{"team-a"},
		Silences:          []SilenceRule{{Namespaces: []string{"batch"}}},
		Templates:         map[string]string{"error": "t"},
		Runbooks:          map[string]string{"OOMKilled": "r"},
		SeverityByReason:  map[string]string{"OOMKilled": "critical"},
		ActiveProbeMonitor: ActiveProbeMonitor{
			HTTP: []HTTPProbeTarget{{Name: "api"}},
		},
	}
	runtime := CompileRuntimeConfig(cfg)

	runtime.Scope().AllowedNamespaces()[0] = "changed"
	runtime.Scope().Silences()[0].Namespaces[0] = "changed"
	runtime.Delivery().Templates()["error"] = "changed"
	runtime.Policy().Runbooks()["OOMKilled"] = "changed"
	runtime.Policy().SeverityByReason()["OOMKilled"] = "changed"
	runtime.ActiveProbe().HTTP[0].Name = "changed"
	cfg.AllowedNamespaces[0] = "changed"

	require.Equal(t, "team-a", runtime.Scope().AllowedNamespaces()[0])
	require.Equal(t, "batch", runtime.Scope().Silences()[0].Namespaces[0])
	require.Equal(t, "t", runtime.Delivery().Templates()["error"])
	require.Equal(t, "r", runtime.Policy().Runbooks()["OOMKilled"])
	require.Equal(t, "critical",
		runtime.Policy().SeverityByReason()["OOMKilled"])
	require.Equal(t, "api", runtime.ActiveProbe().HTTP[0].Name)
}

func TestRuntimeConfigForCompilesOnce(t *testing.T) {
	cfg := &Config{App: App{ClusterName: "a"}}
	require.False(t, RuntimeConfigFor(nil).Compiled())
	require.Equal(t, "a", RuntimeConfigFor(cfg).Application().ClusterName)

	cfg.Runtime = CompileRuntimeConfig(cfg)
	cfg.App.ClusterName = "b"
	require.Equal(t, "a", RuntimeConfigFor(cfg).Application().ClusterName)
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
