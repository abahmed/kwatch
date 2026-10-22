package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
)

func proConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.ActiveProbeMonitor.Enabled = true
	cfg.AuditLog.Output = "/var/log/kwatch/audit.log"
	cfg.HeartbeatMonitor = config.HeartbeatMonitor{
		Enabled: true, URL: "https://hc.example.com/x"}
	cfg.SeverityByReason = map[string]string{"OOMKilled": "critical"}
	cfg.SeverityByOwnerKind = map[string]string{"StatefulSet": "critical"}
	cfg.Maintenance.Annotation = "acme.io/maintenance"
	cfg.Runbooks = map[string]string{"OOMKilled": "https://wiki/oom"}
	cfg.Reasons = []string{"OOMKilled"}
	cfg.AllowedReasons = []string{"OOMKilled"}
	cfg.NamespaceSelector = "team=a"
	cfg.Namespaces = []string{"shop", "!kube-system"}
	cfg.AllowedNamespaces = []string{"shop"}
	cfg.ForbiddenNamespaces = []string{"kube-system"}
	cfg.Silences = []config.SilenceRule{{Namespaces: []string{"dev"}}}
	cfg.Runtime = config.CompileRuntimeConfig(cfg)
	return cfg
}

func TestDefaultConfigurationSetsNoProFeature(t *testing.T) {
	assert.Empty(t, configuredPro(config.DefaultConfig()))
	assert.Empty(t, configuredPro(nil))
}

func TestConfiguredProListsEveryProFeatureTheFileSets(t *testing.T) {
	assert.Equal(t, []feature.ID{
		feature.ActiveProbes, feature.AuditLog, "delivery.heartbeat",
		feature.Maintenance, "policy.namespace-selector",
		"policy.reason-filter", feature.Runbooks,
		feature.SeverityOverrides,
	}, configuredPro(proConfig()))
}

func TestInactiveProIsEmptyForPro(t *testing.T) {
	assert.Empty(t, inactivePro(feature.ProEntitlements(), proConfig()))
}

func TestNilEntitlementsFailClosedToCommunity(t *testing.T) {
	cfg := proConfig()
	assert.Equal(t, inactivePro(feature.CommunityEntitlements(), cfg),
		inactivePro(nil, cfg))
	assert.False(t, entitled(nil, feature.AuditLog))
	assert.True(t, entitled(nil, feature.NodeDetection))

	got := config.RuntimeConfigFor(restrictConfig(cfg, nil))
	assert.Empty(t, got.Policy().SeverityByReason())
	assert.False(t, got.ActiveProbe().Enabled)
}

func TestInactiveProNamesWhatCommunityIgnores(t *testing.T) {
	assert.Equal(t, []string{
		"core.detection.probes", "delivery.audit-log",
		"delivery.heartbeat", "policy.maintenance",
		"policy.namespace-selector", "policy.reason-filter",
		"policy.runbooks", "policy.severity",
	}, inactivePro(feature.CommunityEntitlements(), proConfig()))
}

func TestCommunityIgnoresProPolicyButKeepsScope(t *testing.T) {
	cfg := proConfig()
	got := restrictConfig(cfg, feature.CommunityEntitlements())
	rt := config.RuntimeConfigFor(got)

	assert.Empty(t, rt.Policy().SeverityByReason())
	assert.Empty(t, rt.Policy().SeverityByOwnerKind())
	assert.Empty(t, rt.Policy().Runbooks())
	assert.False(t, rt.Policy().Maintenance().Enabled)
	assert.Empty(t, rt.Scope().AllowedReasons())
	assert.Empty(t, rt.Scope().NamespaceSelector())
	assert.False(t, rt.ActiveProbe().Enabled)
	assert.False(t, rt.Lifecycle().AuditLog().Enabled)
	assert.False(t, rt.Lifecycle().Heartbeat().Enabled)

	assert.Equal(t, []string{"shop"}, rt.Scope().AllowedNamespaces())
	assert.Equal(t, []string{"kube-system"}, rt.Scope().ForbiddenNamespaces())
	assert.Len(t, rt.Scope().Silences(), 1, "silences still work")
	assert.NotEmpty(t, cfg.SeverityByReason, "the original is untouched")
}

func TestProKeepsTheWholeConfiguration(t *testing.T) {
	cfg := proConfig()
	got := restrictConfig(cfg, feature.ProEntitlements())
	rt := config.RuntimeConfigFor(got)

	assert.NotEmpty(t, rt.Policy().SeverityByReason())
	assert.NotEmpty(t, rt.Policy().Runbooks())
	assert.True(t, rt.Policy().Maintenance().Enabled)
	assert.Equal(t, "team=a", rt.Scope().NamespaceSelector())
	assert.True(t, rt.ActiveProbe().Enabled)
	assert.True(t, rt.Lifecycle().Heartbeat().Enabled)
}

func TestCommunityStartupIsNeverAnError(t *testing.T) {
	assert.NotPanics(t, func() {
		logInactivePro(feature.CommunityEntitlements(),
			inactivePro(feature.CommunityEntitlements(), proConfig()))
		logInactivePro(feature.CommunityEntitlements(), nil)
	})
}

func TestCommunityRegistersOnlyPodAndNodeDetectors(t *testing.T) {
	community := registryLayout(
		newDetectorRegistry(nil, feature.CommunityEntitlements(), nil))
	pro := registryLayout(
		newDetectorRegistry(nil, feature.ProEntitlements(), nil))
	assert.Less(t, len(community), len(pro))
	assert.NotEmpty(t, community, "pods and nodes are Community")
	joined := strings.Join(community, "\n")
	assert.Contains(t, joined, "node 0 detectors.restricted",
		"node detection is limited to NotReady")
	assert.Contains(t, joined, "container 0 detectors.restricted",
		"readiness probe failures are Pro")
	for _, pro := range []string{"Workload", "Service", "Claim", "HPA",
		"Certificate", "ClusterService", "NodeUsage", "ActiveProbe"} {
		assert.NotContains(t, joined, "detectors."+pro)
	}
}

func TestStandbyLogsOnceInEveryEdition(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    feature.Entitlements
		want bool
	}{
		{"community", feature.CommunityEntitlements(), true},
		{"pro", feature.ProEntitlements(), true},
		{"unset", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := &serverDeps{entitlements: tc.e,
				healthServer: health.NewHealthServerWithClock(
					config.HealthCheck{}, clock.RealClock{})}
			c := &leaderCallbacks{deps: deps, identity: "me"}

			c.onNewLeader("other")
			c.onNewLeader("other")

			assert.Equal(t, tc.want, c.standbyWarned.Load())
		})
	}
}

func TestServerDepsAreCommunityByDefault(t *testing.T) {
	assert.False(t, (&serverDeps{}).has(feature.AuditLog))
	assert.True(t, (&serverDeps{}).has(feature.NodeDetection))
	assert.False(t, (&serverDeps{}).has(feature.Flapping))
	pro := &serverDeps{entitlements: feature.ProEntitlements()}
	assert.True(t, pro.has(feature.AuditLog))
	deps := &serverDeps{entitlements: feature.CommunityEntitlements()}
	assert.False(t, deps.has(feature.AuditLog))
	assert.True(t, deps.has(feature.NodeDetection))
}

func TestDepsCarryTheEditionFromBootstrap(t *testing.T) {
	e := feature.CommunityEntitlements()
	deps := newServerDeps(t.Context(), func() {}, &bootstrap{
		entitlements: e, inactivePro: []string{"policy.runbooks"},
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{}),
		clock: clock.RealClock{},
	})
	assert.Equal(t, e, deps.entitlements)
	assert.Equal(t, []string{"policy.runbooks"}, deps.inactivePro)
}

func TestIncidentConfigWithoutEntitlementsIsCommunity(t *testing.T) {
	cfg := (&pipelineBuild{deps: &serverDeps{}}).incidentConfig()
	assert.True(t, cfg.NoFlapping)
	assert.True(t, cfg.NoPaging)
	assert.True(t, cfg.NoReminders)
	assert.True(t, cfg.NoHandover)
	assert.Empty(t, cfg.SeverityByReason)

	pro := (&pipelineBuild{deps: &serverDeps{
		entitlements: feature.ProEntitlements(),
		runtime:      config.RuntimeConfigFor(config.DefaultConfig()),
	}}).incidentConfig()
	assert.False(t, pro.NoFlapping)
}
