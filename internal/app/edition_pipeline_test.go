package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

func editionBuild(
	t *testing.T, e feature.Entitlements, cfg *config.Config,
) *pipelineBuild {
	t.Helper()
	deps := pipelineDeps(t, cfg)
	deps.entitlements = e
	p, err := newPipelineBuild(deps, openTestStore(t))
	require.NoError(t, err)
	t.Cleanup(p.close)
	return p
}

func TestCommunityStartsNoFlightRecorder(t *testing.T) {
	assert.Nil(t, editionBuild(t, feature.CommunityEntitlements(),
		&config.Config{}).flight)
	assert.NotNil(t, editionBuild(t, feature.ProEntitlements(),
		&config.Config{}).flight)
}

func TestCommunityLifecycleHasNoFlappingPagingRemindersOrHandover(
	t *testing.T,
) {
	cfg := &config.Config{
		SeverityByReason: map[string]string{"OOMKilled": "critical"},
	}
	community := editionBuild(t, feature.CommunityEntitlements(), cfg).
		incidentConfig()
	assert.True(t, community.NoFlapping)
	assert.True(t, community.NoPaging)
	assert.True(t, community.NoReminders)
	assert.True(t, community.NoHandover)
	assert.Empty(t, community.SeverityByReason)

	pro := editionBuild(t, feature.ProEntitlements(), cfg).incidentConfig()
	assert.False(t, pro.NoFlapping || pro.NoPaging || pro.NoReminders ||
		pro.NoHandover)
	assert.Equal(t, cfg.SeverityByReason, pro.SeverityByReason)
}

func TestCommunityWriterOmitsDetailAndRunbooks(t *testing.T) {
	cfg := &config.Config{
		Runbooks: map[string]string{"OOMKilled": "https://wiki/oom"},
	}
	community := editionBuild(t, feature.CommunityEntitlements(), cfg).
		newWriter()
	assert.True(t, community.Community)
	assert.True(t, community.ProNote)
	assert.Empty(t, community.Runbooks)

	pro := editionBuild(t, feature.ProEntitlements(), cfg).newWriter()
	assert.False(t, pro.Community)
	assert.Len(t, pro.Runbooks, 1)
}

func TestHideProHintsRemovesTheNote(t *testing.T) {
	cfg := &config.Config{App: config.App{HideProHints: true}}
	writer := editionBuild(t, feature.CommunityEntitlements(), cfg).
		newWriter()
	assert.True(t, writer.Community)
	assert.False(t, writer.ProNote)
}

func TestCommunityCollectsNoInvestigationOutput(t *testing.T) {
	assert.Nil(t, editionBuild(t, feature.CommunityEntitlements(),
		&config.Config{}).newInvestigator())
	assert.NotNil(t, editionBuild(t, feature.ProEntitlements(),
		&config.Config{}).newInvestigator())
}

func TestCommunityStartsNoActiveProber(t *testing.T) {
	cfg := &config.Config{
		ActiveProbeMonitor: config.ActiveProbeMonitor{Enabled: true},
	}
	community := pipelineDeps(t, cfg)
	community.entitlements = feature.CommunityEntitlements()
	_, ok := newActiveProber(community, nil, nil)
	assert.False(t, ok)

	pro := pipelineDeps(t, cfg)
	pro.entitlements = feature.ProEntitlements()
	_, ok = newActiveProber(pro, nil, nil)
	assert.True(t, ok)
}

func TestCommunityRunsOnlyTheSourcesItNeeds(t *testing.T) {
	counts := map[string]int{}
	for name, e := range map[string]feature.Entitlements{
		"community": feature.CommunityEntitlements(),
		"pro":       feature.ProEntitlements(),
	} {
		p := editionBuild(t, e, &config.Config{})
		engine, err := p.newEngine()
		require.NoError(t, err)
		runners, err := p.newRunners(engine)
		require.NoError(t, err)
		counts[name] = len(runners)
	}
	// Pro adds the kubelet stats, the control plane prober and the
	// flight recorder.
	assert.Equal(t, counts["community"]+3, counts["pro"])
}

func TestCommunityEngineBaselinesAndHidesProStatus(t *testing.T) {
	p := editionBuild(t, feature.CommunityEntitlements(), &config.Config{})
	p.deps.inactivePro = []string{"policy.runbooks"}
	engine, err := p.newEngine()
	require.NoError(t, err)

	report := engine.Status()

	assert.Equal(t, "Community", report.Edition)
	assert.Equal(t, []string{"policy.runbooks"}, report.ProInactive)
	assert.True(t, report.Brief)
}

func TestProEngineStatusKeepsEverySection(t *testing.T) {
	p := editionBuild(t, feature.ProEntitlements(), &config.Config{})
	engine, err := p.newEngine()
	require.NoError(t, err)

	report := engine.Status()

	assert.Equal(t, "Pro", report.Edition)
	assert.False(t, report.Brief)
}
